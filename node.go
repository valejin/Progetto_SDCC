package main

import (
	"Progetto_SDCC/gossip"
	"sync"
	"time"
)

// NodeStateWithTime estende il NodeState generato con un timestamp locale
// per il rilevamento dei guasti. Questo campo non viene trasmesso via rete.
type NodeStateWithTime struct {
	*gossip.NodeState
	LastUpdated time.Time
}

// Node rappresenta la nostra istanza del servizio.
type Node struct {
	SelfAddr string
	// La chiave è l'indirizzo del nodo, il valore è lo stato del nodo con timestamp.
	MembershipList map[string]NodeStateWithTime
	// Mappa per le lapidi. [addr] -> tempo di rimozione
	Tombstones map[string]time.Time
	mu         sync.RWMutex // Mutex che permette a molti di leggere contemporaneamente, ma solo uno di scrivere.
	seeds      []string
}

// NewNode crea una nuova istanza del nodo, e gli assegna un indirizzo e una lista di seed nodes.
func NewNode(addr, port string, seeds []string) *Node {
	selfAddr := addr + ":" + port

	n := &Node{
		SelfAddr:       selfAddr,
		MembershipList: make(map[string]NodeStateWithTime),
		Tombstones:     make(map[string]time.Time), // Mappa per le lapidi, per gestire i nodi morti.
		seeds:          seeds,
	}

	// Inizializza lo stato del nodo stesso.
	selfState := NodeStateWithTime{
		NodeState: &gossip.NodeState{
			Addr:      selfAddr,
			Status:    gossip.NodeStatus_ALIVE,
			Heartbeat: 1,
		},
		LastUpdated: time.Now(),
	}
	n.MembershipList[selfAddr] = selfState

	return n
}
