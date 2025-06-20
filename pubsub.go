package main

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

const CHANNEL_BUFFER_SIZE = 100
const SLOW_SUBSCRIBER_TIMEOUT = 50 * time.Millisecond

var ALLOWED_SUBSCRIBED_MODE_COMMANDS = []string{"UNSUBSCRIBE", "SUBSCRIBE", "RESET"}

type PubSub struct {
	mu               sync.RWMutex
	openChannels     map[string][]*Subscriber
	channelsByWorker map[string][]string
}

type Subscriber struct {
	channel   chan string
	id        string
	closeOnce sync.Once
}

func NewPubSub() *PubSub {
	return &PubSub{mu: sync.RWMutex{}, openChannels: map[string][]*Subscriber{}, channelsByWorker: make(map[string][]string)}
}

func NewSubscriber(channel chan string, id string) *Subscriber {
	return &Subscriber{channel: channel, id: id, closeOnce: sync.Once{}}
}

func (s *Subscriber) Close() {
	s.closeOnce.Do(func() {
		close(s.channel)
	})
}

func publishMessagesToSubscribers(message string, subscribers []*Subscriber) {
	for _, subscriber := range subscribers {
		select {
		case subscriber.channel <- message:
		default:
			select {
			case subscriber.channel <- message:
			case <-time.After(SLOW_SUBSCRIBER_TIMEOUT):
				subscriber.Close()
			}
		}
	}
}

func writeChannelMessages(channel chan string, channelName string, writer *Writer) {
	for msg := range channel {
		writer.Write(Value{Type: "array", array: []Value{
			{Type: "bulk", bulk: "message"},
			{Type: "bulk", bulk: channelName},
			{Type: "bulk", bulk: msg},
		}})
	}
}

func (pubsub *PubSub) Publish(args []Value, ctx context.Context, writer *Writer) {
	if len(args) != 2 {
		writer.Write(Value{Type: "error", str: "ERR wrong number of arguments for 'publish' command"})
		return
	}

	channelName := args[0].bulk
	pubsub.mu.RLock()
	defer pubsub.mu.RUnlock()

	subscribers, exists := pubsub.openChannels[channelName]

	if !exists {
		writer.Write(Value{Type: "integer", num: 0})
		return
	}

	subsCopy := make([]*Subscriber, len(subscribers))
	copy(subsCopy, subscribers)

	go publishMessagesToSubscribers(args[1].bulk, subsCopy)
	writer.Write(Value{Type: "integer", num: len(subsCopy)})
}

func (pubsub *PubSub) Subscribe(args []Value, ctx context.Context, writer *Writer) {
	if len(args) < 1 {
		writer.Write(Value{Type: "error", str: "ERR wrong number of arguments for 'subscribe' command"})
		return
	}

	pubsub.mu.Lock()

	clientState := ctx.Value(clientStateKey{}).(*ClientState)
	id := clientState.ID

	if _, exists := pubsub.channelsByWorker[id]; !exists {
		pubsub.channelsByWorker[id] = []string{}
	}

	clientState.IsSubscribed = true

	workerChannels := pubsub.channelsByWorker[id]

	var subscriptionGoroutines []struct {
		ch chan string
		cn string
	}

	for _, arg := range args {
		channelName := arg.bulk

		subscriber := NewSubscriber(make(chan string, CHANNEL_BUFFER_SIZE), id)

		if _, exists := pubsub.openChannels[channelName]; !exists {
			pubsub.openChannels[channelName] = []*Subscriber{subscriber}
		} else {
			pubsub.openChannels[channelName] = append(pubsub.openChannels[channelName], subscriber)
		}

		if !slices.Contains(workerChannels, channelName) {
			workerChannels = append(workerChannels, channelName)

			pubsub.channelsByWorker[id] = workerChannels
		}

		writer.Write(Value{Type: "array", array: []Value{
			{Type: "bulk", bulk: "subscribe"},
			{Type: "bulk", bulk: channelName},
			{Type: "integer", num: len(workerChannels)},
		}})

		subscriptionGoroutines = append(subscriptionGoroutines, struct {
			ch chan string
			cn string
		}{ch: subscriber.channel, cn: channelName})
	}
	pubsub.mu.Unlock()

	for _, sub := range subscriptionGoroutines {
		go writeChannelMessages(sub.ch, sub.cn, writer)
	}
}

func (pubsub *PubSub) Unsubscribe(args []Value, ctx context.Context, writer *Writer) {
	if len(args) < 1 {
		writer.Write(Value{Type: "error", str: "ERR wrong number of arguments for 'unsubscribe' command"})
		return
	}

	pubsub.mu.Lock()
	defer pubsub.mu.Unlock()

	clientState := ctx.Value(clientStateKey{}).(*ClientState)
	id := clientState.ID

	if _, exists := pubsub.channelsByWorker[id]; !exists {
		return
	}

	workerChannels := pubsub.channelsByWorker[id]

	for _, arg := range args {
		channelName := arg.bulk

		if !slices.Contains(workerChannels, channelName) {
			continue
		}

		channelNameIndex := slices.Index(workerChannels, channelName)
		workerChannels = slices.Delete(workerChannels, channelNameIndex, channelNameIndex+1)
		pubsub.channelsByWorker[id] = workerChannels

		subscribers, exists := pubsub.openChannels[channelName]
		if exists {
			idx := slices.IndexFunc(subscribers, func(s *Subscriber) bool { return s.id == id })

			if idx != -1 {
				subscriber := subscribers[idx]
				subscriber.Close()
				pubsub.openChannels[channelName] = slices.Delete(subscribers, idx, idx+1)

				if len(pubsub.openChannels[channelName]) == 0 {
					delete(pubsub.openChannels, channelName)
				}
			}
		}

		writer.Write(Value{Type: "array", array: []Value{
			{Type: "bulk", bulk: "unsubscribe"},
			{Type: "bulk", bulk: channelName},
			{Type: "integer", num: len(workerChannels)},
		}})
	}

	clientState.IsSubscribed = len(workerChannels) > 0

	if len(workerChannels) == 0 {
		delete(pubsub.channelsByWorker, id)
	}
}

func (pubsub *PubSub) Reset(args []Value, ctx context.Context, writer *Writer) {
	if len(args) != 0 {
		writer.Write(Value{Type: "error", str: "ERR wrong number of arguments for 'reset' command"})
		return
	}

	clientState := ctx.Value(clientStateKey{}).(*ClientState)
	id := clientState.ID

	if clientState.IsSubscribed {
		workerChannels := pubsub.channelsByWorker[id]

		if len(workerChannels) > 0 {
			var unsubscribeArgs = []Value{}

			for _, channelName := range workerChannels {
				unsubscribeArgs = append(unsubscribeArgs, Value{Type: "string", bulk: channelName})
			}

			pubsub.Unsubscribe(unsubscribeArgs, ctx, writer)
		}
	}

	writer.Write(Value{Type: "string", str: "RESET"})
}

func (pubsub *PubSub) ValidateCommand(command string, ctx context.Context, writer *Writer) bool {
	if (!ctx.Value(clientStateKey{}).(*ClientState).IsSubscribed) {
		return true
	}

	if !slices.Contains(ALLOWED_SUBSCRIBED_MODE_COMMANDS, strings.ToUpper(command)) {
		writer.Write(Value{Type: "error", str: "ERR this command is not allowed in subscribed mode"})
		return false
	}

	return true
}

type PubSubHandlerFunc func(args []Value, ctx context.Context, writer *Writer)

func (ps *PubSub) GetHandlers() map[string]PubSubHandlerFunc {
	return map[string]PubSubHandlerFunc{
		"PUBLISH":     ps.Publish,
		"SUBSCRIBE":   ps.Subscribe,
		"UNSUBSCRIBE": ps.Unsubscribe,
		"RESET":       ps.Reset,
	}
}
