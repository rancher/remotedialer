package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var (
	targetHost = "api-extension.cattle-system.svc.cluster.local"
	targetPort = 6666
	retryDelay = 5 * time.Second
)

func init() {
	if host, ok := os.LookupEnv("TARGET_HOST"); ok {
		targetHost = host
	}

	if portStr, ok := os.LookupEnv("TARGET_PORT"); ok {
		if p, err := strconv.Atoi(portStr); err != nil {
			fmt.Printf("Could not parse TARGET_PORT=%q: %v. Using default %d.\n",
				portStr, err, targetPort)
		} else {
			targetPort = p
		}
	}

	if intervalStr, ok := os.LookupEnv("SEND_INTERVAL"); ok {
		if i, err := strconv.Atoi(intervalStr); err != nil {
			fmt.Printf("Could not parse SEND_INTERVAL=%q: %v. Using default %v.\n",
				intervalStr, err, retryDelay)
		} else {
			retryDelay = time.Duration(i) * time.Second
		}
	}
}

func echoHandler(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	go func() {
		<-ctx.Done()
		fmt.Println("echoHandler: context canceled; closing connection.")
		_ = conn.Close()
	}()

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("Connection closed or error occurred: %v\n", err)
			return
		}

		fmt.Println("Received from Server:", string(buffer[:n]))

		// Echo back the received data
		if _, err := conn.Write(buffer[:n]); err != nil {
			fmt.Printf("Error sending data back: %v\n", err)
			return
		}

		fmt.Println("Sent back to Server:", string(buffer[:n]))
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("main: received shutdown signal; canceling context...")
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("main: context canceled; exiting dial loop.")
			return
		default:
		}

		fmt.Printf("Attempting to connect to %s:%d...\n", targetHost, targetPort)

		conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", targetHost, targetPort))
		if err != nil {
			fmt.Printf("Failed to connect: %v. Retrying in %v...\n", err, retryDelay)
			time.Sleep(retryDelay)
			continue
		}

		fmt.Println("Connected to the server.")

		// Send a welcome message
		welcomeMessage := "Hello, server! Client has connected.\nPlease type any word and hit enter:"
		if _, err = conn.Write([]byte(welcomeMessage)); err != nil {
			fmt.Printf("Error sending welcome message: %v\n", err)
			conn.Close()
			continue
		}

		echoHandler(ctx, conn)
	}
}
