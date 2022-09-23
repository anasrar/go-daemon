package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/akamensky/argparse"
)

// in case you want to make something like broadcast message
var socketClients map[string]*net.Conn = map[string]*net.Conn{}

func main() {
	root := argparse.NewParser("go-daemon", "Daemon in go with TCP.")

	host := root.String("", "host", &argparse.Options{
		Required: false,
		Help:     "Host",
		Default:  "localhost",
	})
	port := root.String("p", "port", &argparse.Options{
		Required: false,
		Help:     "Port",
		Default:  "9876",
	})

	start := root.NewCommand("start", "Start daemon server")

	noDaemon := start.Flag("n", "nodaemon", &argparse.Options{
		Required: false,
		Help:     "Start server without daemon.",
		Default:  false,
	})

	stop := root.NewCommand("stop", "Stop daemon server")

	ping := root.NewCommand("ping", "Send ping to server")

	if err := root.Parse(os.Args); err != nil {
		fmt.Print(root.Usage(err))
		return
	}

	switch {

	case start.Happened():
		if !*noDaemon {
			// check current process is daemon using env variable
			daemon := false
			flag := "APP_DAEMON=true"
			for _, env := range os.Environ() {
				if daemon = env == flag; daemon {
					break
				}
			}

			if !daemon {
				binary, _ := os.Executable()
				process, err := os.StartProcess(binary, os.Args, &os.ProcAttr{
					Env: append([]string{flag}, os.Environ()...),
				})
				if err != nil {
					log.Print("daemon start error: ", err)
				}
				if err := process.Release(); err != nil {
					log.Print("daemon detach error: ", err)
				}
				return
			}
		}

		socketServer := NewSocketServer(*host, *port)
		defer socketServer.Close()
		log.Print("socket server listen on " + *host + ":" + *port)

	NEW_CLIENT_LOOP:
		for {
			socketClient, err := socketServer.Accept()
			if err != nil {
				switch {
				case errors.Is(err, net.ErrClosed):
					log.Print("socket server close")
					break NEW_CLIENT_LOOP

				default:
					log.Print("socket server accept client error: ", err)
					continue NEW_CLIENT_LOOP
				}
			}

			socketClients[socketClient.RemoteAddr().String()] = &socketClient
			log.Print("socket server: new client")

			go SocketServerHandleClientMessage(&socketServer, &socketClient)
		}

	case stop.Happened():
		socketClient := NewSocketClient(*host, *port)
		defer socketClient.Close()

		socketClient.Write([]byte("stop"))

	case ping.Happened():
		socketClient := NewSocketClient(*host, *port)
		defer socketClient.Close()

		socketClient.Write([]byte("ping"))

		var wg sync.WaitGroup
		wg.Add(1)
		go SocketClientHandleServerMessage(&socketClient, &wg)
		wg.Wait()

	}

}

func NewSocketServer(host string, port string) net.Listener {
	socketServer, err := net.Listen("tcp", host+":"+port)
	if err != nil {
		log.Fatal("socket server error: ", err)
	}

	return socketServer
}

func SocketServerHandleClientMessage(socketServer *net.Listener, socketClient *net.Conn) {
	defer (*socketClient).Close()
	defer func() {
		log.Print("socket server: client disconnect")
		if _, found := socketClients[(*socketClient).RemoteAddr().String()]; found {
			delete(socketClients, (*socketClient).RemoteAddr().String())
		}
	}()

	buffer := make([]byte, 1024)
	data := bufio.NewReader(*socketClient)
	send := bufio.NewWriter(*socketClient)

NEW_MESSAGE_LOOP:
	for {
		length, err := data.Read(buffer)
		message := string(buffer[:length])

		switch err {

		case io.EOF: // error occur when client close
			break NEW_MESSAGE_LOOP

		case nil:
			log.Print("socket server get message: ", message)

			switch message {

			case "stop":
				(*socketServer).Close()
				break NEW_MESSAGE_LOOP

			case "ping":
				send.Write([]byte("pong"))
				send.Flush()

			}

		default:
			log.Print("socket server message loop error: ", err)
			break NEW_MESSAGE_LOOP

		}
	}
}

func NewSocketClient(host string, port string) net.Conn {
	socketClient, err := net.Dial("tcp", host+":"+port)
	if err != nil {
		log.Fatal("socket client error: ", err)
	}

	return socketClient
}

func SocketClientHandleServerMessage(socketClient *net.Conn, wg *sync.WaitGroup) {
	defer wg.Done()

	buffer := make([]byte, 1024)
	data := bufio.NewReader(*socketClient)

NEW_MESSAGE_LOOP:
	for {
		length, err := data.Read(buffer)
		message := string(buffer[:length])

		switch err {
		case io.EOF: // error occur when server close
			break NEW_MESSAGE_LOOP

		case nil:
			log.Print("socket client get message: ", message)

			switch message {

			case "pong":
				break NEW_MESSAGE_LOOP

			}

		default:
			log.Print("socket client message loop error: ", err)
			break NEW_MESSAGE_LOOP
		}
	}
}
