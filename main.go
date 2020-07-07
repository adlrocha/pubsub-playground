package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (n *Node) log(msg string) {
	log.Printf("[%s] %s \n", n.host.ID().Pretty(), msg)
}

func main() {
	// // parse some flags to set our nickname and the room to join
	// nickFlag := flag.String("nick", "", "nickname to use in chat. will be generated if empty")
	// roomFlag := flag.String("room", "awesome-chat-room", "name of chat room to join")
	// flag.Parse()

	numPublishers := flag.Int("pubs", 2, "Number of publishers to start")
	numSubscribers := flag.Int("subs", 20, "Number of subscribers to start")
	startTracerNode := flag.Bool("tracer", false, "Start a tracer node")
	flag.Parse()

	fmt.Println("Cleaning previous traces...")
	os.RemoveAll("./traces/")
	os.Mkdir("./traces/", 0755)

	// Start new tracer node
	if *startTracerNode {
		idch := make(chan string, 1)
		go initTracer(idch)
		tracerID := <-idch
		fmt.Println("Initialized new tracer node with ID", tracerID)
	}

	// Starting publishers
	log.Println("Creating publishers...")
	publishers := make([]*Node, *numPublishers)
	for i := 0; i < *numPublishers; i++ {
		publishers[i] = createNode(true, "")
	}

	// Create subscribers and subscribe to topics randomly.
	log.Println("Creating and starting subscribers...")
	subscribers := make([]*Node, *numSubscribers)
	for i := 0; i < *numSubscribers; i++ {
		subscribers[i] = createNode(false, "")
		rand.Seed(time.Now().UnixNano())
		randNum := rand.Intn(*numPublishers)
		subscribers[i].start(publishers[randNum].host.ID().Pretty())
	}

	log.Println("Starting publishers and starting publishing messages...")
	for i := 0; i < *numPublishers; i++ {
		publishers[i].start("")
	}

	log.Println("Start reading logs...")
	startReadingLogs()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		// TODO: Close peers
		os.Exit(0)
	}
	// TODO: Manage contexts.
}
