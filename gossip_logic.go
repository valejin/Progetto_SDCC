package main

import (
	"Progetto_SDCC/gossip"
	"context"
	"encoding/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"
)

// Questo file contiene la logica di gossip del nodo, decide quando agire e cosa fare.

const (
	gossipInterval   = 1 * time.Second
	failureInterval  = 3 * time.Second
	suspectTimeout   = 5 * time.Second
	cleanupTimeout   = 10 * time.Second
	snapshotInterval = 15 * time.Second // Salva lo stato su disco ogni 15 secondi
	tombstoneTimeout = 60 * time.Second // Dopo quanto una lapide può essere rimossa
	kRandomPeers     = 1                // Numero di peer casuali da contattare per il gossip
)

// --- FUNZIONE DI BOOTSTRAP: tenta di connettersi ai seed. ---
// Restituisce 'true' se si connette ad almeno un seed, altrimenti 'false'.
func (n *Node) bootstrap() bool {
	if len(n.seeds) == 0 {
		log.Println("➡️Nessun seed fornito, avvio come primo nodo del cluster.")
		return true // Non è un fallimento, è una scelta.
	}

	log.Println("➡️Avvio del processo di bootstrap contattando i seed in sequenza:", strings.Join(n.seeds, ", "))

	// Itera sui seed uno per uno.
	for _, seedAddr := range n.seeds {
		log.Printf("➡️Tento di contattare il seed: %s", seedAddr)

		// Impostiamo un breve timeout per ogni singolo tentativo di connessione.
		// In questo modo non rimaniamo bloccati a lungo su un seed irraggiungibile.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		conn, err := grpc.DialContext(ctx, seedAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		cancel() // Liberiamo le risorse del contesto non appena abbiamo finito.

		if err != nil {
			log.Printf("❌ Impossibile connettersi al seed %s: %v. Provo il prossimo.", seedAddr, err)
			continue // Se la connessione fallisce, passa al prossimo seed nel ciclo.
		}

		// Se la connessione ha successo, eseguiamo la chiamata RPC.
		client := gossip.NewGossipServiceClient(conn)

		n.mu.RLock()
		selfState := n.MembershipList[n.SelfAddr]
		req := &gossip.GossipRequest{
			SenderAddr: n.SelfAddr,
			MembershipList: map[string]*gossip.NodeState{
				n.SelfAddr: selfState.NodeState,
			},
		}
		n.mu.RUnlock()

		// Chiamata RPC con un suo timeout.
		rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err := client.ShareState(rpcCtx, req)
		rpcCancel()

		// Chiudiamo la connessione, non ci serve più.
		conn.Close()

		if err != nil {
			log.Printf("❌ Errore nella chiamata RPC al seed %s: %v. Provo il prossimo.", seedAddr, err)
			continue // Se la chiamata RPC fallisce, passa al prossimo seed.
		}

		// SUCCESSO!
		// Abbiamo contattato un seed e ricevuto una risposta valida.
		log.Printf("✅ Connesso con successo al seed %s. Ricevuta lista membri. Bootstrap completato.", seedAddr)
		n.mergeLists(resp.MembershipList, seedAddr)

		return true // Usciamo immediatamente dalla funzione e dal ciclo. Missione compiuta.
	}

	// Se il ciclo 'for' finisce, significa che abbiamo provato tutti i seed senza successo.
	log.Println("AVVISO: Impossibile contattare qualsiasi seed dopo averli provati tutti. Il nodo opererà in isolamento.")
	return false
}

// StartGossipLoop è il ciclo principale del nodo.
func StartGossipLoop(node *Node) {
	node.bootstrap()

	// Inizializza i ticker per il gossip e il controllo dei fallimenti.
	gossipTicker := time.NewTicker(gossipInterval)
	failureTicker := time.NewTicker(failureInterval)
	snapshotTicker := time.NewTicker(snapshotInterval)
	defer gossipTicker.Stop()
	defer failureTicker.Stop()
	defer snapshotTicker.Stop()

	//log.Println("Avvio del ciclo di gossip e monitoraggio...")
	for {
		select {
		case <-gossipTicker.C:
			node.doGossip()
		case <-failureTicker.C:
			node.checkFailures()
		case <-snapshotTicker.C:
			log.Println("💾Salvataggio periodico dello stato su file...")
			node.saveStateToFile() // Chiamiamo una funzione dedicata al salvataggio
		}
	}
}

// doGossip esegue un round di gossip.
func (n *Node) doGossip() {
	n.mu.Lock()
	selfState := n.MembershipList[n.SelfAddr]
	selfState.Heartbeat++ // Incrementa il heartbeat per il nodo stesso
	selfState.LastUpdated = time.Now()
	n.MembershipList[n.SelfAddr] = selfState // Aggiorna lo stato del nodo stesso
	n.mu.Unlock()

	peers := n.selectRandomPeers(kRandomPeers) // Seleziona k peer casuali per il gossip
	for _, peer := range peers {
		go n.sendGossipRPC(peer)
	}
}

// mergeLists fonde la lista remota (formato gRPC) con quella locale.
func (n *Node) mergeLists(remoteList map[string]*gossip.NodeState, sourceAddr string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Se un'informazione è nuova o più aggiornata (con un heartbeat più alto), la aggiorniamo.
	for addr, remoteState := range remoteList {

		if _, isTombstoned := n.Tombstones[addr]; isTombstoned {
			// C'è una lapide per questo nodo.

			// ECCEZIONE: Se l'informazione remota dice che il nodo è tornato VIVO,
			// allora è una resurrezione legittima. Dobbiamo accettarla
			// e rimuovere la lapide per permettergli di rientrare.
			if remoteState.Status == gossip.NodeStatus_ALIVE {
				log.Printf("🟢Nodo %s è tornato vivo, rimuovo la sua lapide.", addr)
				delete(n.Tombstones, addr)
				// Ora procediamo con la normale logica di merge per questo nodo.
			} else {
				// Altrimenti, se non è ALIVE, è solo un pettegolezzo vecchio su un nodo
				// che abbiamo deciso di dimenticare. Ignoriamolo.
				continue
			}
		}

		localState, exists := n.MembershipList[addr]

		// NUOVA LOGICA DI LOGGING
		if !exists {
			// Questo è un nodo completamente nuovo per noi.
			if addr == sourceAddr {
				log.Printf("🤝 Scoperto NUOVO nodo direttamente: %s si è presentato.", addr)
			} else {
				log.Printf("👂 Scoperto NUOVO nodo indirettamente: %s mi ha parlato di %s.", sourceAddr, addr)
			}
		}

		// --- LOGICA DI MERGE ---

		// Se non esiste, lo aggiungiamo sempre.
		if !exists {
			n.MembershipList[addr] = NodeStateWithTime{NodeState: remoteState, LastUpdated: time.Now()}
			continue // Passa al prossimo
		}

		// Se esiste, la logica è più complessa:
		// REGOLA 1: Se lo stato locale è DEAD, ignora tutto tranne un heartbeat più alto.
		if localState.Status == gossip.NodeStatus_DEAD {
			// L'unico modo per uscire da DEAD è una vera resurrezione che implica uno stato ALIVE.
			// Seguo la logica delle lapidi per i riavvii, e qui ignoro qualsiasi informazione che non abbia un heartbeat maggiore.
			// Questo previene la "falsa resurrezione".
			if remoteState.Heartbeat <= localState.Heartbeat {
				continue
			}
		}

		// REGOLA 2: L'heartbeat più alto vince sempre (tranne per il caso DEAD gestito sopra).
		if remoteState.Heartbeat > localState.Heartbeat {
			// Un caso speciale: se un nodo torna ALIVE dopo essere stato SUSPECT, lo accettiamo.
			isRevivedFromSuspect := localState.Status == gossip.NodeStatus_SUSPECT && remoteState.Status == gossip.NodeStatus_ALIVE

			n.MembershipList[addr] = NodeStateWithTime{NodeState: remoteState, LastUpdated: time.Now()}

			if isRevivedFromSuspect {
				log.Printf("✅ Nodo %s è tornato ALIVE da SUSPECT. (da %s)", addr, sourceAddr)
			} else {
				log.Printf("🚀 Stato aggiornato per %s -> Status: %s, Heartbeat: %d (da %s)", addr, remoteState.Status, remoteState.Heartbeat, sourceAddr)
			}
		}
	}
}

// selectRandomPeers sceglie k peer casuali.
func (n *Node) selectRandomPeers(k int) []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	var peers []string
	for addr, state := range n.MembershipList {
		if addr != n.SelfAddr && state.Status == gossip.NodeStatus_ALIVE {
			peers = append(peers, addr)
		}
	}
	rand.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
	if len(peers) > k {
		return peers[:k]
	}
	return peers
}

// markAsSuspect cambia lo stato di un nodo a Suspect.
func (n *Node) markAsSuspect(addr string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if state, ok := n.MembershipList[addr]; ok && state.Status != gossip.NodeStatus_DEAD {
		state.Status = gossip.NodeStatus_SUSPECT
		state.LastUpdated = time.Now()
		n.MembershipList[addr] = state
		log.Printf("❗ Nodo %s marcato come SUSPECT\n", addr)
	}
}

func (n *Node) saveStateToFile() {
	// Usiamo RLock perché stiamo solo leggendo per salvare.
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Usiamo una struct temporanea per un output JSON più pulito.
	type stateForJSON struct {
		SelfAddr       string                       `json:"self_address"`
		MembershipList map[string]*gossip.NodeState `json:"membership_list"`
	}

	// Creiamo la mappa da salvare, convertendo il nostro stato interno.
	listToSave := make(map[string]*gossip.NodeState)
	for addr, stateWithTime := range n.MembershipList {
		listToSave[addr] = stateWithTime.NodeState
	}

	state := stateForJSON{
		SelfAddr:       n.SelfAddr,
		MembershipList: listToSave,
	}

	// Marshalling (conversione da struct a JSON) con indentazione per leggibilità.
	fileData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Printf("❌ Errore durante la serializzazione dello stato in JSON: %v\n", err)
		return
	}

	// Creiamo un nome di file univoco per ogni nodo basato sul suo indirizzo.
	// Sostituiamo ":" con "_" per avere un nome file valido.
	filename := "stato_" + strings.Replace(n.SelfAddr, ":", "_", -1) + ".json"

	err = os.WriteFile(filename, fileData, 0644)
	if err != nil {
		log.Printf("❌ Errore durante il salvataggio dello stato su file %s: %v\n", filename, err)
	} else {
		log.Printf("💾Stato salvato con successo nel file %s\n", filename)
	}
}

// checkFailures controlla i nodi e li marca come Dead o li rimuove.
func (n *Node) checkFailures() {
	n.mu.Lock()
	//defer n.mu.Unlock()

	// Flag per sapere se c'è stato un cambiamento nella lista dei membri
	stateChanged := false

	for addr, state := range n.MembershipList {
		if addr == n.SelfAddr {
			continue
		}
		// Se il nodo è Suspect e il timeout è scaduto, lo marciamo come Dead.
		if state.Status == gossip.NodeStatus_SUSPECT && time.Since(state.LastUpdated) > suspectTimeout {
			state.Status = gossip.NodeStatus_DEAD
			state.LastUpdated = time.Now()
			n.MembershipList[addr] = state
			log.Printf("❗ Nodo %s marcato come DEAD\n", addr)
			stateChanged = true
		}
		// Se il nodo è Dead e il timeout di cleanup è scaduto, lo rimuoviamo dalla lista.
		if state.Status == gossip.NodeStatus_DEAD && time.Since(state.LastUpdated) > cleanupTimeout {
			log.Printf("➡️Rimuovo il nodo morto %s dalla lista e creo una lapide.\n", addr)
			delete(n.MembershipList, addr)
			n.Tombstones[addr] = time.Now()
			stateChanged = true
		}
	}

	// Pulizia delle lapidi: rimuoviamo le lapidi più vecchie di tombstoneTimeout.
	for addr, t := range n.Tombstones {
		if time.Since(t) > tombstoneTimeout {
			log.Printf("➡️Rimuovo la lapide per il nodo %s.", addr)
			delete(n.Tombstones, addr)
		}
	}

	// Rilasciamo il lock di scrittura IMMEDIATAMENTE dopo aver finito le modifiche.
	n.mu.Unlock()

	log.Println("--- ✨ Lista Membri✨ ---")
	n.mu.RLock() // Acquisiamo un lock di sola lettura, che è più permissivo
	for addr, state := range n.MembershipList {
		log.Printf("  - %s | Status: %s | Heartbeat: %d", addr, state.Status, state.Heartbeat)
	}
	n.mu.RUnlock()
	log.Println("--------------------")

	// Salviamo lo stato su file solo se è effettivamente cambiato qualcosa.
	if stateChanged {
		log.Println("💾Lo stato è cambiato, salvo su file...")
		n.saveStateToFile()
	}
}
