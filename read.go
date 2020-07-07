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

type ReportingData struct {
	PeerList       map[string]bool
	TopicList      map[string]bool
	Topics         int64
	Peers          int64
	PublishedMsg   int64
	DeliveredMsg   int64
	DuplicateMsg   int64
	SentRPC        int64
	MessageDelays  map[string]*DelayData
	TotalMsgDelay  float64
	TotalDelaysNum int64
	mux            *sync.Mutex
}

type DelayData struct {
	publishTimestamp  float64
	deliverTimestamps []float64
}

func startReadingLogs() {
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
	go ds.startReporting()
}

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
		// TODO: Process for each line the log.
		if len(data) != 0 {
			ds.processData(data)
		}

		if err == io.EOF {
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
		// Compute average delay
		for _, m := range ds.MessageDelays {
			for _, v := range m.deliverTimestamps {
				ds.TotalMsgDelay += (v - m.publishTimestamp)
				ds.TotalDelaysNum++
			}
		}

		avgDelay := float64(ds.TotalMsgDelay) / float64(ds.TotalDelaysNum)

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
