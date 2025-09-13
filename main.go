package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	port := flag.String("port", "8080", "Porta su cui il nodo si mette in ascolto")
	addr := flag.String("addr", "127.0.0.1", "Indirizzo IP che il nodo pubblicizza")
	seedsStr := flag.String("seeds", "", "Lista di indirizzi di seed node separati da virgola (es. 127.0.0.1:8080)")
	flag.Parse()

	var seeds []string
	if *seedsStr != "" {
		seeds = strings.Split(*seedsStr, ",")
	}

	node := NewNode(*addr, *port, seeds)

	// Avvia il server gRPC in una goroutine
	go StartGRPCServer(node, *port)

	// Avvia il ciclo di gossip in un'altra goroutine
	go StartGossipLoop(node)

	// Logica di arresto grazioso
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)
	<-shutdownChan

	log.Println("Ricevuto segnale di arresto. Avvio del graceful shutdown...")

	node.Shutdown()

	// --- OPERAZIONI DI CLEANUP ---

	// Salva l'ultimo stato su file.
	log.Println("💾Salvataggio dello stato finale su file...")
	node.saveStateToFile() // Chiamiamo la funzione di salvataggio

	time.Sleep(1 * time.Second)
	log.Println("🔴Arresto del nodo completato.")

}
