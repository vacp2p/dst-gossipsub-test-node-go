package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	lcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	ma "github.com/multiformats/go-multiaddr"
)

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	return hostname
}

func getEnvVariables() (int, int, int, int) {
	PEERS, err := strconv.Atoi(os.Getenv("PEERS"))

	if err != nil {
		fmt.Println("Error converting string to integer:", err)
		panic(err)
	}

	CONNECTTO, err := strconv.Atoi(os.Getenv("CONNECTTO"))

	if err != nil {
		fmt.Println("Error converting string to integer:", err)
		panic(err)
	}

	MSGRATE, err := strconv.Atoi(os.Getenv("MSGRATE"))

	if err != nil {
		fmt.Println("Error converting string to integer:", err)
		panic(err)
	}

	MSGSIZE, err := strconv.Atoi(os.Getenv("MSGSIZE"))

	if err != nil {
		fmt.Println("Error converting string to integer:", err)
		panic(err)
	}

	return PEERS, CONNECTTO, MSGRATE, MSGSIZE
}

func generateKey(podName string) lcrypto.PrivKey {
	hash := sha256.Sum256([]byte(podName))
	p, err := crypto.ToECDSA(hash[:])
	if err != nil {
		panic(err)
	}
	privK, _ := btcec.PrivKeyFromBytes(p.D.Bytes())
	if err != nil {
		panic(err)
	}
	key := (*lcrypto.Secp256k1PrivateKey)(privK)

	libp2pPrivkey := lcrypto.PrivKey(key)

	return libp2pPrivkey
}

func makeHost(podName string) (host.Host, error) {
	sourceMultiAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/5000")

	pk := generateKey(podName)

	return libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Identity(pk),
	)
}

func extractID(s string) (int, error) {
	numberStr := s[len("pod-"):]

	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return 0, err
	}

	return number, nil
}

func getPeersToConnect(N int) []int {
	numbers := make([]int, N)
	for i := 0; i < N; i++ {
		numbers[i] = i
	}

	rand.Shuffle(N, func(i, j int) {
		numbers[i], numbers[j] = numbers[j], numbers[i]
	})

	return numbers
}

func resolveAddress(addr string) (net.IP, error) {
	ips, err := net.LookupIP(addr)
	if err != nil {
		return nil, err
	}

	ipAddr := ips[0]
	return ipAddr, nil
}

func readIDsFile() map[string]interface{} {
	fileContent, err := os.ReadFile("ids.json")
	if err != nil {
		fmt.Println("Error reading the JSON file:", err)
		return nil
	}

	var data map[string]interface{}

	err = json.Unmarshal(fileContent, &data)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return nil
	}

	return data
}

func readLoop(sub *pubsub.Subscription, ctx context.Context) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			continue
		}
		receivedTimestamp := binary.LittleEndian.Uint64(msg.Data[:8])
		receivedTime := time.Unix(0, int64(receivedTimestamp))
		currentTimestamp := time.Now().UnixNano()
		currentTime := time.Unix(0, currentTimestamp)
		timeDifference := currentTime.Sub(receivedTime).Milliseconds()
		fmt.Printf("%s, milliseconds: %d\n", receivedTime, timeDifference)
	}
}

func createGSParams() pubsub.GossipSubParams {
	gsParams := pubsub.DefaultGossipSubParams()

	gsParams.D = 6
	gsParams.Dlo = 4
	gsParams.Dhi = 8
	gsParams.Dscore = 6
	gsParams.Dout = 3
	gsParams.Dlazy = 6
	gsParams.HeartbeatInterval = time.Second
	gsParams.PruneBackoff = time.Minute
	gsParams.GossipFactor = 0.25

	return gsParams
}

func main() {
	ctx := context.Background()
	p2pIDs := readIDsFile()
	hostname := getHostname()

	id, err := extractID(hostname)

	PEERS, CONNECTTO, MSGRATE, MSGSIZE := getEnvVariables()

	gsParams := createGSParams()

	h, err := makeHost(hostname)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Hostname: %s, ID: %s\n", hostname, h.ID())

	ps, err := pubsub.NewGossipSub(ctx, h, pubsub.WithGossipSubParams(gsParams))
	if err != nil {
		println("Error starting pubsub protocol", err)
		panic(err)
	}

	topic, _ := ps.Join("test")

	topicScoreParams := pubsub.TopicScoreParams{
		TopicWeight:                  1,
		FirstMessageDeliveriesWeight: 1,
		FirstMessageDeliveriesCap:    3,
		MeshMessageDeliveriesDecay:   0.9,
	}

	_ = topic.SetScoreParams(&topicScoreParams)

	sub, err := topic.Subscribe()
	if err != nil {
		println(err)
	}

	go readLoop(sub, ctx)

	println("Waiting 30 seconds for node building...")
	time.Sleep(time.Second * 30)

	peersToConnect := getPeersToConnect(PEERS)

	connections := 0
	for i := 0; i < len(peersToConnect); i++ {
		if connections >= CONNECTTO {
			break
		}
		if peersToConnect[i] == id {
			continue
		}
		fmt.Printf("Will connect to peer %d\n", peersToConnect[i])
		fmt.Printf("Service: %d\n", peersToConnect[i])

		iString := strconv.Itoa(peersToConnect[i])
		tAddress := fmt.Sprintf("pod-%d", peersToConnect[i])

		fmt.Printf("Trying to resolve %s\n", tAddress)

		var ipString string
		for {
			ip, err := resolveAddress(tAddress)
			if err != nil {
				fmt.Printf("Failed to resolve address: %s\n", tAddress)
				println("Waiting 15 seconds...")
				time.Sleep(time.Second * 15)
			} else {
				println("Resolved!")
				fmt.Printf("%s resolved: %s\n", tAddress, ip.String())
				ipString = ip.String()
				break
			}
		}

		for {
			fmt.Printf("Trying to connect to %s\n", ipString)
			nodeIDString := p2pIDs["pod-"+iString]
			nodeID, _ := peer.Decode(nodeIDString.(string))
			mAddrs := "/ip4/" + ipString + "/tcp/5000"
			fmt.Printf("Creating multiaddres from %s\n", mAddrs)
			multiAddrs, mErr := ma.NewMultiaddr(mAddrs)
			if mErr != nil {
				fmt.Printf("Failed to create multiaddress, %s\n", err)
			}
			multiaddrsArray := []ma.Multiaddr{multiAddrs}

			info := peer.AddrInfo{
				ID:    nodeID,
				Addrs: multiaddrsArray,
			}
			err := h.Connect(ctx, info)
			if err != nil {
				fmt.Printf("Failed to dial, %s\n", err)
				println("Waiting 15 seconds...")
				time.Sleep(time.Second * 15)
			} else {
				println("Conected!")
				connections += 1
				break
			}
		}
	}

	println("Waiting 30 seconds for node connections...")
	time.Sleep(time.Second * 30)

	peers := ps.ListPeers("test")
	fmt.Printf("Mesh size: %d\n", len(peers))
	fmt.Printf("Publishing turn is: %d\n", id)

	counter := 0
	for {
		time.Sleep(time.Duration(MSGRATE) * time.Millisecond)
		if counter%PEERS == id {
			now := time.Now().UnixNano()

			nowBytes := make([]byte, 8)
			binary.LittleEndian.PutUint64(nowBytes, uint64(now))

			var nowBytesExtended = append(nowBytes, make([]byte, MSGSIZE)...)
			err := topic.Publish(ctx, nowBytesExtended)
			if err != nil {
				fmt.Printf("Error publishing: %d\n", err)
			} else {
				fmt.Printf("Sent message at: %d\n", now)
			}
		}
		counter += 1
	}
}
