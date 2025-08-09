package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
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

	// --- LOGICA DI ARRESTO GRAZIOSO ---
	// Creiamo un canale per ricevere il segnale di sistema
	shutdown := make(chan os.Signal, 1)
	// Notifichiamo al canale quando riceviamo SIGINT (Ctrl+C) o SIGTERM
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	// Blocchiamo l'esecuzione qui finché non riceviamo un segnale
	<-shutdown

	// --- OPERAZIONI DI CLEANUP ---
	log.Println("Arresto del nodo...")
	// Salva l'ultimo stato su file.
	log.Println("💾Salvataggio dello stato finale su file...")
	node.saveStateToFile() // Chiamiamo la funzione di salvataggio

	log.Println("🔴Arresto del nodo completato.")

}
