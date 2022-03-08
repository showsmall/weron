package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

var (
	errMissingPath        = errors.New("missing path")
	errMissingCommunityID = errors.New("missing community ID")
	errMissingPassword    = errors.New("missing password")
	errWrongPassword      = errors.New("wrong password")

	upgrader = websocket.Upgrader{}
)

type input struct {
	messageType int
	p           []byte
}

type community struct {
	password string
	conns    map[string]*websocket.Conn
}

func main() {
	laddr := flag.String("laddr", ":1337", "Listening address")
	heartbeat := flag.Duration("heartbeat", time.Second*10, "Time to wait for heartbeats")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()

	addr, err := net.ResolveTCPAddr("tcp", *laddr)
	if err != nil {
		panic(err)
	}

	if port := os.Getenv("PORT"); port != "" {
		if *verbose {
			log.Println("Using port from PORT env variable")
		}

		p, err := strconv.Atoi(port)
		if err != nil {
			panic(err)
		}

		addr.Port = p
	}

	log.Println("Listening on address", addr.String())

	var communitiesLock sync.Mutex
	communities := map[string]community{}

	panic(
		http.ListenAndServe(
			addr.String(),
			http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				defer func() {
					if err := recover(); err != nil {
						log.Println("closed connection for client with address", r.RemoteAddr+":", err)
					}
				}()

				path := strings.Split(r.URL.Path, "/")
				if len(path) < 1 {
					panic(errMissingPath)
				}

				communityID := path[len(path)-1]
				if strings.TrimSpace(communityID) == "" {
					panic(errMissingCommunityID)
				}

				password := r.URL.Query().Get("password")
				if strings.TrimSpace(password) == "" {
					panic(errMissingPassword)
				}
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				if err != nil {
					panic(err)
				}

				conn, err := upgrader.Upgrade(rw, r, nil)
				if err != nil {
					panic(err)
				}

				defer func() {
					communitiesLock.Lock()
					delete(communities[communityID].conns, r.RemoteAddr)
					if len(communities[communityID].conns) <= 0 {
						delete(communities, communityID)
					}
					communitiesLock.Unlock()

					log.Println("Disconnected from client with address", r.RemoteAddr, "in community", communityID)

					if err := conn.Close(); err != nil {
						panic(err)
					}
				}()

				communitiesLock.Lock()
				if _, exists := communities[communityID]; !exists {
					communities[communityID] = community{
						password: string(hashedPassword),
						conns:    map[string]*websocket.Conn{},
					}
				}
				if bcrypt.CompareHashAndPassword([]byte(communities[communityID].password), []byte(password)) != nil {
					communitiesLock.Unlock()

					panic(errWrongPassword)
				}
				communities[communityID].conns[r.RemoteAddr] = conn
				communitiesLock.Unlock()

				log.Println("Connected to client with address", r.RemoteAddr, "in community", communityID)

				if err := conn.SetReadDeadline(time.Now().Add(*heartbeat)); err != nil {
					panic(err)
				}
				conn.SetPongHandler(func(string) error {
					return conn.SetReadDeadline(time.Now().Add(*heartbeat))
				})

				pings := time.NewTicker(*heartbeat / 2)
				defer pings.Stop()

				inputs := make(chan input)
				errs := make(chan error)
				go func() {
					for {
						messageType, p, err := conn.ReadMessage()
						if err != nil {
							if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
								errs <- err
							}

							errs <- nil

							return
						}

						if *verbose {
							log.Println("Received message with type", messageType, "from client with address", r.RemoteAddr, "in community", communityID)
						}

						inputs <- input{messageType, p}
					}
				}()

				for {
					select {
					case err := <-errs:
						panic(err)
					case input := <-inputs:
						for id, conn := range communities[communityID].conns {
							if id == r.RemoteAddr {
								continue
							}

							if *verbose {
								log.Println("Sending message with type", input.messageType, "to client with address", r.RemoteAddr, "in community", communityID)
							}

							if err := conn.WriteMessage(input.messageType, input.p); err != nil {
								panic(err)
							}

							if err := conn.SetWriteDeadline(time.Now().Add(*heartbeat)); err != nil {
								panic(err)
							}
						}
					case <-pings.C:
						if *verbose {
							log.Println("Sending ping to client with address", r.RemoteAddr, "in community", communityID)
						}

						if err := conn.SetWriteDeadline(time.Now().Add(*heartbeat)); err != nil {
							panic(err)
						}

						if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
							panic(err)
						}
					}
				}
			}),
		),
	)
}
