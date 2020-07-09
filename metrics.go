package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"
)

// ReportingData that we want to track.
type ReportingData struct {
	PeerList      map[string]bool       // List of seen peers
	TopicList     map[string]bool       // List of topics seen
	Topics        int64                 // Number of topics
	Peers         int64                 // Number of peers
	PublishedMsg  int64                 // Number of published messages in th network
	DeliveredMsg  int64                 // Number of delivered messages to subscribed nodes
	DuplicateMsg  int64                 // Duplicate messages received. Determines level redundancy
	SentRPC       int64                 // Number of RPC sent
	MessageDelays map[string]*DelayData // Timestamps of pubslihed and received timestamps for every message published
	AvgDelay      float64               // AverageDelay of messages published and delivered in the network.
	mux           *sync.Mutex
}

// DelayData data structure for each message
type DelayData struct {
	publishTimestamp  float64
	deliverTimestamps []float64
}

// Start reading metrics from log
func startReadingLogs(logReport bool, server bool) {
	tracesDir := "./traces/"
	files, err := ioutil.ReadDir(tracesDir)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Starting reporting data...")
	ds := ReportingData{
		PeerList:      make(map[string]bool),
		TopicList:     make(map[string]bool),
		MessageDelays: map[string]*DelayData{},
		mux:           &sync.Mutex{},
	}

	for _, f := range files {
		fmt.Println("Reading: ", tracesDir+f.Name())
		go ds.readLog(tracesDir + f.Name())
	}

	time.Sleep(5 * time.Second)

	// Start reporting in console.
	if logReport {
		go ds.startReporting()
	}
	// Start server websocket and dashboard
	if server {
		ds.startServer()

	}
}

// Read pubs and subs logs line by line.
func (ds *ReportingData) readLog(path string) {
	input, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err = input.Close(); err != nil {
			log.Fatal(err)
		}
	}()
	r := bufio.NewReader(input)
	for {
		data, err := r.ReadBytes('\n')
		if len(data) != 0 {
			// Process line of the log
			ds.processData(data)
		}

		if err == io.EOF {
			// If no new logs to process wait for a bit
			time.Sleep(time.Second)
		} else if err != nil {
			panic(err)
		}
	}
}

func (ds *ReportingData) processData(data []byte) {
	dataMap := make(map[string]interface{})
	json.Unmarshal(data, &dataMap)

	// Prepare data in advance to avoid blocking other routines.
	peerID := dataMap["peerID"].(string)
	var subList []interface{}
	if dataMap["recvRPC"] != nil {
		if dataMap["recvRPC"].(map[string]interface{})["meta"].(map[string]interface{})["subscription"] != nil {
			subList = dataMap["recvRPC"].(map[string]interface{})["meta"].(map[string]interface{})["subscription"].([]interface{})
		}
	}
	// Sync ready
	ds.mux.Lock()
	defer ds.mux.Unlock()

	// Count peers seen
	// (Maybe it would be interesting to add a decay to account for node churn)
	if !ds.PeerList[peerID] {
		ds.PeerList[peerID] = true
		ds.Peers++
	}

	// Count topics seen.
	// As before we should maybe account for disappearing topics.
	if len(subList) != 0 {
		for _, v := range subList {
			vtopic := v.(map[string]interface{})["topic"].(string)
			if !ds.TopicList[vtopic] {
				ds.TopicList[vtopic] = true
				ds.Topics++
			}
		}
	}

	// Count published messages
	if dataMap["publishMessage"] != nil {

		// Update delays
		msgID := dataMap["publishMessage"].(map[string]interface{})["messageID"].(string)
		if ds.MessageDelays[msgID] == nil {
			ds.MessageDelays[msgID] = &DelayData{
				publishTimestamp:  0,
				deliverTimestamps: make([]float64, 0),
			}
		}
		ds.MessageDelays[msgID].publishTimestamp = dataMap["timestamp"].(float64)
		ds.PublishedMsg++
	}

	// Count delivered messages
	if dataMap["deliverMessage"] != nil {

		// Deliver messages
		msgID := dataMap["deliverMessage"].(map[string]interface{})["messageID"].(string)
		// Update delays
		if ds.MessageDelays[msgID] == nil {
			ds.MessageDelays[msgID] = &DelayData{
				publishTimestamp:  0,
				deliverTimestamps: make([]float64, 0),
			}
		}
		ds.MessageDelays[msgID].deliverTimestamps = append(ds.MessageDelays[msgID].deliverTimestamps, dataMap["timestamp"].(float64))
		ds.DeliveredMsg++
	}

	// Count duplicate messages
	if dataMap["duplicateMessage"] != nil {
		ds.DuplicateMsg++
	}

	// Count sent RPCs
	if dataMap["sendRPC"] != nil {
		ds.SentRPC++
	}
}

func (ds *ReportingData) startReporting() {
	for {
		var totalMsgDelay, totalDelaysNum float64
		totalMsgDelay, totalDelaysNum = 0, 0
		// Compute average delay
		for _, m := range ds.MessageDelays {
			for _, v := range m.deliverTimestamps {
				totalMsgDelay += (v - m.publishTimestamp)
				totalDelaysNum++
			}
		}

		avgDelay := float64(totalMsgDelay) / float64(totalDelaysNum)

		// Update in ds to send it through WS
		ds.mux.Lock()
		ds.AvgDelay = avgDelay
		ds.mux.Unlock()

		fmt.Println("=====================")
		fmt.Printf("Peers: %d\n", ds.Peers)
		fmt.Printf("Topics: %d\n", ds.Topics)
		fmt.Printf("PublishedMsg: %d\n", ds.PublishedMsg)
		fmt.Printf("DeliveredMsg: %d\n", ds.DeliveredMsg)
		fmt.Printf("DuplicateMsg: %d\n", ds.DuplicateMsg)
		fmt.Printf("SentRPC: %d\n", ds.SentRPC)
		fmt.Printf("Average Delay: %f ms\n", avgDelay/1000000)
		fmt.Println("=====================")
		time.Sleep(5 * time.Second)
	}
}
