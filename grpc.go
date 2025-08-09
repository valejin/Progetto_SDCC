package main

import (
	"Progetto_SDCC/gossip"
	"context"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Questo file implementa sia il lato Server (ascoltare le chiamate degli altri)
// sia il lato Client (chiamare gli altri) della comunicazione gRPC.

// gossipServer è la nostra implementazione del server gRPC.
// Deve avere un riferimento al nostro nodo per accedere alla lista dei membri.
type gossipServer struct {
	gossip.UnimplementedGossipServiceServer // Embedding obbligatorio per compatibilità futura
	node                                    *Node
}

// ShareState è l'implementazione del metodo RPC definito nel .proto.
func (s *gossipServer) ShareState(ctx context.Context, req *gossip.GossipRequest) (*gossip.GossipResponse, error) {
	// Riceviamo una lista remota, la mergiamo con la nostra
	s.node.mergeLists(req.MembershipList)

	// Prepariamo la risposta con la nostra lista aggiornata
	s.node.mu.RLock()
	defer s.node.mu.RUnlock()

	responseList := make(map[string]*gossip.NodeState)
	for addr, state := range s.node.MembershipList {
		responseList[addr] = state.NodeState
	}

	return &gossip.GossipResponse{MembershipList: responseList}, nil
}

// StartGRPCServer avvia il server gRPC in ascolto.
func StartGRPCServer(node *Node, port string) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("❌ Impossibile mettersi in ascolto sulla porta %s: %v", port, err)
	}

	// Crea un nuovo server gRPC
	grpcServer := grpc.NewServer()

	// Registra la nostra implementazione del servizio
	gossip.RegisterGossipServiceServer(grpcServer, &gossipServer{node: node})

	log.Printf("📡Server gRPC in ascolto su %s\n", lis.Addr().String())
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("❌ Errore nell'avvio del server gRPC: %v", err)
	}
}

// sendGossipRPC è la funzione per inviare gossip usando gRPC, è la parte client.
func (n *Node) sendGossipRPC(peerAddr string) {
	if peerAddr == n.SelfAddr {
		return
	}

	// Creiamo una connessione gRPC al peer, ossia tenta di stabilire una connessione con l'altro peer.
	// grpc.WithBlock() fa sì che Dial blocchi fino a quando la connessione non è stabilita o fallisce.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		// Se la connessione fallisce, sospettiamo che il nodo sia down
		log.Printf("❌ Impossibile connettersi a %s: %v. Marco come Suspect.", peerAddr, err)
		n.markAsSuspect(peerAddr)
		return
	}
	defer conn.Close()

	// Crea un client stub, un oggetto che ci permette di chiamare le funzioni remote come se fossero locali.
	client := gossip.NewGossipServiceClient(conn)

	// Prepara la richiesta con la nostra lista di membri
	n.mu.RLock()
	requestList := make(map[string]*gossip.NodeState)
	for addr, state := range n.MembershipList {
		requestList[addr] = state.NodeState
	}
	n.mu.RUnlock()

	req := &gossip.GossipRequest{MembershipList: requestList}

	// Esegui la chiamata RPC
	resp, err := client.ShareState(context.Background(), req)
	if err != nil {
		log.Printf("❌ Errore nella chiamata RPC a %s: %v. Marco come Suspect.", peerAddr, err)
		n.markAsSuspect(peerAddr)
		return
	}

	// Se la chiamata ha successo, mergiamo la lista ricevuta in risposta
	n.mergeLists(resp.MembershipList)
}
