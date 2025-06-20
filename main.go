package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
)

var goroutineCounter uint64

type clientStateKey struct{}

type ClientState struct {
	ID           string
	IsSubscribed bool
}

func GetInitialClientState() *ClientState {
	id := atomic.AddUint64(&goroutineCounter, 1)
	clientState := &ClientState{
		ID:           strconv.FormatUint(id, 10),
		IsSubscribed: false,
	}

	return clientState
}

func NewContext() context.Context {
	clientState := GetInitialClientState()

	ctx := context.WithValue(context.Background(), clientStateKey{}, clientState)

	return ctx
}

func handleConnection(conn net.Conn, aof *Aof, ctx context.Context, pubsub *PubSub) {
	defer conn.Close()

	for {
		resp := NewResp(conn)
		value, err := resp.Read()
		if err != nil {
			fmt.Println(err)
			return
		}

		if value.Type != "array" {
			fmt.Println("Invalid request, expected array")
			continue
		}

		if len(value.array) == 0 {
			fmt.Println("Invalid request, expected array length > 0")
			continue
		}

		command := strings.ToUpper(value.array[0].bulk)
		args := value.array[1:]

		writer := NewWriter(conn)

		handler, ok := Handlers[command]
		if !ok {
			isValid := pubsub.ValidateCommand(command, ctx, writer)
			if !isValid {
				continue
			}

			pubsubHandler, exists := pubsub.GetHandlers()[command]

			if !exists {
				fmt.Println("Invalid command: ", command)
				writer.Write(Value{Type: "string", str: ""})
				continue
			}

			pubsubHandler(args, ctx, writer)
			continue
		}

		if command == "SET" || command == "HSET" {
			aof.Write(value)
		}

		result := handler(args)
		writer.Write(result)
	}
}
func main() {
	pubsub := NewPubSub()
	fmt.Println("HI")

	fmt.Println("Listening on port :6379")

	// Create a new server
	l, err := net.Listen("tcp", ":6379")
	if err != nil {
		fmt.Println(err)
		return
	}

	aof, err := NewAof("database.aof")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer aof.Close()

	aof.Read(func(value Value) {
		command := strings.ToUpper(value.array[0].bulk)
		args := value.array[1:]

		handler, exists := Handlers[command]
		if !exists {
			fmt.Println("Invalid command: ", command)
			return
		}

		handler(args)
	})

	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}

		ctx := NewContext()
		go handleConnection(conn, aof, ctx, pubsub)
	}
}
