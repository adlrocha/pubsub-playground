package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	ma "github.com/multiformats/go-multiaddr"
)

// Node structure
type Node struct {
	publisher bool
	host      host.Host
	ctx       context.Context
	ps        *pubsub.PubSub
	topic     *pubsub.Topic
	sub       *pubsub.Subscription
}

// PublishMessage simple structure. Could be expanded.
type PublishMessage struct {
	Msg       string
	Timestamp int64
}

// DiscoveryInterval is how often we re-publish our mDNS records.
const DiscoveryInterval = time.Hour

// MsgDeliveryIntervaal time between msg publication for publishers
const MsgDeliveryInterval = 5

// DiscoveryServiceTag is used in our mDNS advertisements to discover other chat peers.
const DiscoveryServiceTag = "pubsub-playground"

func createNode(publisher bool, tracerID string) *Node {
	ctx := context.Background()
	// create a new libp2p Host that listens on a random TCP port
	h, err := libp2p.New(ctx, libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))
	if err != nil {
		panic(err)
	}

	var ps *pubsub.PubSub
	prefix := "sub-"
	if publisher {
		prefix = "pub-"
	}
	// If no tracerID it means we are using JSON Tracer
	if tracerID == "" {
		traceFile := fmt.Sprintf("./traces/%s%s.json", prefix, h.ID().Pretty())
		// traceFile := "./traces/out.json"
		tracer, err := pubsub.NewJSONTracer(traceFile)
		if err != nil {
			fmt.Println("Couldn't start tracer")
		}
		// create a new PubSub service using the GossipSub router
		ps, err = pubsub.NewGossipSub(ctx, h, pubsub.WithEventTracer(tracer))
	} else { // If there is tracerID we are using a remote tracer node
		// assuming that your tracer runs in x.x.x.x and has a peer ID of QmTracer
		// TODO: Support for tracer nodes is not completed.
		pi, err := peer.AddrInfoFromP2pAddr(ma.StringCast(tracerID))
		if err != nil {
			panic(err)
		}

		tracer, err := pubsub.NewRemoteTracer(ctx, h, *pi)
		if err != nil {
			panic(err)
		}
		// create a new PubSub service using the GossipSub router
		ps, err = pubsub.NewGossipSub(ctx, h, pubsub.WithEventTracer(tracer))
	}

	if err != nil {
		panic(err)
	}

	// setup local mDNS discovery
	err = setupDiscovery(ctx, h)
	if err != nil {
		panic(err)
	}

	return &Node{
		host:      h,
		publisher: publisher,
		ctx:       ctx,
		ps:        ps,
	}
}

// Start a node with a publisher or subscriber role and subscribed to a single topic.
func (n *Node) start(topic string) {
	if n.publisher {
		var err error
		// create new topic with the id of the publisher
		n.topic, err = n.ps.Join(n.host.ID().Pretty())
		if err != nil {
			panic(err)
		}
		// and subscribe to it
		n.sub, err = n.topic.Subscribe()
		if err != nil {
			panic(err)
		}

		// Start publishing
		go n.publisherLoop()
	} else {
		var err error
		// create new topic
		n.topic, err = n.ps.Join(topic)
		if err != nil {
			panic(err)
		}
		// and subscribe to it
		n.sub, err = n.topic.Subscribe()
		if err != nil {
			panic(err)
		}

		// Start reading messages
		go n.subscriberLoop()
	}
}

// Start publisher loop for a node
func (n *Node) publisherLoop() {
	n.log("Starting publisher loop...")
	select {
	case <-n.ctx.Done():
		n.host.Close()
		return
	default:
		for {
			// Always send the same simple message.
			msg := PublishMessage{
				Timestamp: time.Now().UnixNano(),
				Msg:       fmt.Sprintf("Hello from %s", n.host.ID().Pretty()),
			}
			n.log("Publishing new message...")
			msgBytes, _ := json.Marshal(msg)
			// Publish message to topic
			n.topic.Publish(n.ctx, msgBytes)
			time.Sleep(MsgDeliveryInterval * time.Second)
		}
	}
}

// Start subscriber loop for a node.
func (n *Node) subscriberLoop() {
	n.log("Starting subscriber loop...")
	select {
	case <-n.ctx.Done():
		n.host.Close()
		return
	default:
		for {
			msg, err := n.sub.Next(n.ctx)
			if err != nil {
				n.log(fmt.Sprintf("Received error in subscriber: %v", err))
			}
			if msg.ReceivedFrom == n.host.ID() {
				n.log("This message is mine")
			}
			cm := new(PublishMessage)
			err = json.Unmarshal(msg.Data, cm)
			if err != nil {
				continue
			}
		}
	}
}

// discoveryNotifee gets notified when we find a new peer via mDNS discovery
type discoveryNotifee struct {
	h host.Host
}

// HandlePeerFound connects to peers discovered via mDNS. Once they're connected,
// the PubSub system will automatically start interacting with them if they also
// support PubSub.
func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	// fmt.Printf("discovered new peer %s\n", pi.ID.Pretty())
	err := n.h.Connect(context.Background(), pi)
	if err != nil {
		fmt.Printf("error connecting to peer %s: %s\n", pi.ID.Pretty(), err)
	}
}

// setupDiscovery creates an mDNS discovery service and attaches it to the libp2p Host.
// This lets us automatically discover peers on the same LAN and connect to them.
func setupDiscovery(ctx context.Context, h host.Host) error {
	// setup mDNS discovery to find local peers
	disc, err := discovery.NewMdnsService(ctx, h, DiscoveryInterval, DiscoveryServiceTag)
	if err != nil {
		return err
	}

	n := discoveryNotifee{h: h}
	disc.RegisterNotifee(&n)
	return nil
}
