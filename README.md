# Go Redis Implementation

A Redis server implementation in Go with zero external dependencies, based on the tutorial at [build-redis-from-scratch.dev](https://www.build-redis-from-scratch.dev/en/introduction). The original tutorial covers RESP serialization and basic Redis commands (SET, GET, HSET, HGET). This implementation extends that foundation with **concurrency support using goroutines** for multiple clients and a complete **PUB/SUB system** using Go channels and context for client state management.

## Features

This implementation includes the core Redis functionality with additional enhancements:

### Basic Redis Commands
- **SET** - Store key-value pairs
- **GET** - Retrieve values by key
- **HSET** - Store hash field-value pairs
- **HGET** - Retrieve hash field values
- **PING** - Server connectivity test

### Enhanced Features
- **Concurrency Support** - Multiple client connections handled via goroutines
- **PUB/SUB System** - Real-time messaging with channels
  - `SUBSCRIBE` - Subscribe to channels
  - `UNSUBSCRIBE` - Unsubscribe from channels  
  - `PUBLISH` - Publish messages to channels
  - `RESET` - Reset client state
- **RESP Protocol** - Complete Redis Serialization Protocol implementation
- **AOF Persistence** - Append-only file for data persistence

## Architecture

### Core Components

- **RESP Parser** (`resp.go`) - Handles Redis Serialization Protocol
- **Command Handler** (`handler.go`) - Processes Redis commands with thread-safe operations
- **PUB/SUB System** (`pubsub.go`) - Fan-out messaging using Go channels
- **AOF Persistence** (`aof.go`) - Append-only file for data durability
- **Server** (`main.go`) - TCP server with goroutine-based client handling

### Concurrency Model

- Each client connection runs in its own goroutine
- Thread-safe data structures using `sync.RWMutex`
- Context-based client state management
- Channel-based communication for PUB/SUB

## Getting Started

### Prerequisites

- Go 1.24.0 or later
- No external dependencies required

### Installation

```bash
git clone <repository-url>
cd go-redis
```

### Running the Server

```bash
go run .
```

The server will start on `localhost:6379` (default Redis port).

### Testing with Redis CLI

You can test the server using the standard Redis CLI:

```bash
redis-cli

# Basic commands
127.0.0.1:6379> SET mykey "Hello World"
OK
127.0.0.1:6379> GET mykey
"Hello World"

# Hash commands
127.0.0.1:6379> HSET myhash field1 "value1"
(integer) 1
127.0.0.1:6379> HGET myhash field1
"value1"

# PUB/SUB
127.0.0.1:6379> SUBSCRIBE mychannel
Reading messages... (press Ctrl-C to quit)

# In another terminal
127.0.0.1:6379> PUBLISH mychannel "Hello subscribers!"
(integer) 1
```

## Implementation Details

### RESP Protocol Support

The implementation handles all RESP data types:
- Simple Strings
- Errors
- Integers
- Bulk Strings
- Arrays
- Null values

### Thread Safety

All data structures are protected with appropriate mutexes:
- `SETs` - Key-value store with `sync.RWMutex`
- `HSETs` - Hash store with `sync.RWMutex`
- PUB/SUB channels with `sync.RWMutex`

### PUB/SUB Features

- **Channel multiplexing** - Multiple subscribers per channel
- **Slow subscriber handling** - Automatic cleanup of unresponsive subscribers
- **Buffered channels** - Configurable buffer size for message queuing
- **Context-based state** - Client subscription state tracking

## Project Structure

```
├── main.go         # TCP server and connection handling
├── handler.go      # Redis command implementations
├── resp.go         # RESP protocol parser
├── pubsub.go       # PUB/SUB system implementation
├── aof.go          # Append-only file persistence
├── go.mod          # Go module definition
└── README.md       # This file
```

## Performance Considerations

- Goroutine-per-connection model for high concurrency
- Read-write mutexes for optimal read performance
- Buffered channels to prevent blocking on slow subscribers
- Timeout mechanisms for subscriber cleanup

## Limitations

This is an educational implementation and has several limitations compared to production Redis:

- No clustering support
- Limited command set
- Basic persistence (AOF only)
- No memory optimization
- No authentication
- No configuration options

## Learning Outcomes

This project demonstrates:
- TCP server implementation in Go
- Protocol parsing and serialization
- Concurrent programming with goroutines
- Thread-safe data structures
- Channel-based communication patterns
- Context usage for request-scoped data

## Acknowledgments

Based on the excellent tutorial series at [build-redis-from-scratch.dev](https://www.build-redis-from-scratch.dev/en/introduction) with additional enhancements for concurrency and PUB/SUB functionality.

## License

This project is for educational purposes. Please refer to the original tutorial for licensing information.