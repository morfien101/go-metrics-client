package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	flagAuthHost = flag.String("auth", "http://172.17.0.1:80", "Endpoint for getting Auth information")
	flagWriters  = flag.Int("writers", 5, "number of writers to start")
	flagDebug    = flag.Bool("debug", false, "Enables debug logging")
	flagVersion  = flag.Bool("version", false, "Show the version")
	flagHelp     = flag.Bool("h", false, "Show the help menu")
	VERSION      = "0.0.1"
	client       = http.Client{}
)

type measurement struct {
	Name   string                 `json:"measurement"`
	Fields map[string]interface{} `json:"fields"`
	Tags   map[string]string      `json:"tags"`
}

func main() {
	flag.Parse()
	if *flagHelp {
		flag.PrintDefaults()
		return
	}
	if *flagVersion {
		fmt.Println(VERSION)
		return
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	rand.Seed(time.Now().UnixNano())
	breakout := false

	log.Println("Starting writers")
	senders := make(map[int]chan os.Signal)
	wg := &sync.WaitGroup{}
	for id := 0; id < *flagWriters; id++ {
		stopChan := make(chan os.Signal, 1)
		innerID := id
		senders[innerID] = stopChan
		go func() {
			for {
				if breakout {
					return
				}
				wg.Add(1)
				defer wg.Done()
				username, password, endpoint := getAuth(*flagAuthHost)
				if *flagDebug {
					fmt.Printf("Got Auth:\nUsername: %s\nPassword: %s\nHost: %s\n", username, password, endpoint)
				}
				sendMetricWS(innerID, username, password, endpoint, stopChan)
				log.Printf("gofer-%d stopped and is going to start again.", innerID)
				if !breakout {
					time.Sleep(time.Second)
				}
			}
		}()
	}
	go func() {
		select {
		case sig := <-interrupt:
			breakout = true
			for _, senderInt := range senders {
				senderInt <- sig
			}
		}
	}()
	time.Sleep(time.Second * 1)
	wg.Wait()
}

func sendMetricWS(id int, username, password, endpoint string, interrupt chan os.Signal) {
	log.Println("starting gofer", id)
	makeMeasurement := func() []byte {
		m := &measurement{
			Name: "test_measurement",
			Fields: map[string]interface{}{
				"value": rand.Intn(100),
			},
			Tags: map[string]string{
				"from": fmt.Sprintf("gofer-%d", id),
			},
		}

		b, _ := json.Marshal(m)
		return b
	}

	h := http.Header{
		"Authorization": {
			"Basic " + base64.StdEncoding.EncodeToString(
				[]byte(username+":"+password),
			)}}
	u := url.URL{Scheme: "ws", Host: endpoint, Path: "/metrics"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), h)
	if err != nil {
		log.Println("dial:", err)
		return
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
		}
	}()

	ticker := time.NewTicker(time.Millisecond * 10)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			err := c.WriteMessage(websocket.TextMessage, makeMeasurement())
			if err != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			log.Println(id, "- shutting down")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println(id, "- write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

func getAuth(endpoint string) (string, string, string) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/auth", endpoint), nil)
	if err != nil {
		fmt.Printf("Error creating request. Error: %s\n", err)
		os.Exit(1)
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Got an error making HTTP request. Error:", err)
		os.Exit(1)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body. Error:", err)
	}
	resp.Body.Close()

	type expected struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Endpoint string `json:"endpoint"`
	}
	output := &expected{}
	err = json.Unmarshal(body, output)
	return output.Username, output.Password, output.Endpoint
}
