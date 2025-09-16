Progetto di Service Discovery e Failure Detection basato su Protocollo Gossip
================================================================
Questo progetto è un'implementazione di un sistema di service discovery e failure detection decentralizzato per il corso di Sistemi Distribuiti e Cloud Computing. Il sistema utilizza un protocollo Gossip per permettere a un cluster di nodi di mantenere una visione condivisa e aggiornata dei membri attivi, rilevando guasti e uscite in modo resiliente.

L'implementazione è realizzata in Go (Golang), utilizza gRPC per la comunicazione tra i nodi e viene orchestrata tramite Docker e Docker Compose per una facile gestione e deployment.

Architettura
------------
- **main.go**: Punto di ingresso dell'applicazione, gestisce i flag e l'avvio.
- **node.go**: Definisce le strutture dati principali (Node, NodeState).
- **gossip_logic.go**: Contiene il cuore del protocollo Gossip, la logica di failure detection e lo shutdown.
- **grpc.go**: Implementa il server gRPC e il client per la comunicazione tra nodi.
- **gossip/**: Contiene la definizione del protocollo (.proto) e il codice generato.
- **Dockerfile**: Definisce l'immagine Docker per l'applicazione.
- **docker-compose.yml**: Configura il cluster di nodi per l'esecuzione con Docker Compose.

Caratteristiche Principali
--------------------------------
- **Decentralizzazione**: Nessun single point of failure. Ogni nodo partecipa alla diffusione delle informazioni.
- **Service Discovery**: I nuovi nodi possono unirsi al cluster e scoprire automaticamente gli altri membri.
- **Failure Detection**: I guasti (crash) e le uscite controllate dei nodi vengono rilevati e propagati all'intero cluster.
- **Protocollo Gossip (Push-Pull)**: Comunicazione bidirezionale efficiente per una rapida convergenza dello stato.
- **Gestione dell'Uscita Volontaria**: Un nodo che esce volontariamente notifica il cluster per una rimozione immediata.
- **Resilienza**: Il sistema è tollerante alla perdita di nodi, inclusi i nodi di bootstrap (seed).
- **Persistenza**: Lo stato topologico del cluster viene salvato periodicamente e in seguito a cambiamenti significativi.
- **Containerizzazione**: L'applicazione è completamente containerizzata con Docker per una facile esecuzione su qualsiasi ambiente.

Prerequisiti
----------------
Per eseguire questo progetto, sono necessari i seguenti strumenti:
- **Git**: Per clonare il repository.
- **Go**: Versione 1.23 o successiva.
- **Docker e Docker Compose**: Per costruire l'immagine e orchestrare il cluster di container.

Come Avviare il Progetto (con Docker Compose)
--------------------------------
1. Clona il repository:
   ```bash
   git clone https://github.com/valejin/Progetto_SDCC.git
   cd Progetto_SDCC
    ```
2. Costruisci l'immagine Docker:
   ```bash
   docker build -t gossip-service .
   ```
3. Avvia il cluster di nodi (i nodi vengono avviati in sequenza):
   ```bash
    docker compose up 
    ```
4. Per avviare il cluster in background e visualizzare il log del nodo particolare, usa:
   ```bash
   docker compose up -d
   docker compose logs -f node-d  # Sostituisci 'node-d' con il nome del nodo desiderato
   ```
5. Per fermare il cluster, esegui:
   ```bash
    docker compose down
    ```

Come Testare gli Scenari di Guasto
--------------------------------
- **Test di Uscita Volontaria (Graceful Shutdown)**:
    ```bash
    # Ferma il nodo 'node-c'
    docker compose stop node-c
    ```

    ```bash
    # Riavvia il nodo 'node-c'
    docker compose start node-c
    ```
- **Test di Crash Improvviso**:
    ```bash
    # Ferma il nodo 'node-d' senza preavviso
    docker compose kill node-d
    ```

    ```bash
    # Riavvia il nodo 'node-d'
    docker compose start node-d
    ```

Come Eseguire il Progetto Localmente (Senza Docker)
--------------------------------
1. Clona il repository:
   ```bash
   git clone https://github.com/valejin/Progetto_SDCC.git
   cd Progetto_SDCC
    ```
2. Installa le dipendenze:
   ```bash
   go mod download
   ```
3. Avvia i nodi in terminali separati (ad esempio, per 4 nodi):
   ```bash
   # Terminale 1 (Nodo A - Seed)
   go run . -port=8080
   ```

   ```bash
   # Terminale 2 (Nodo B - Seed)
   go run . -port=8081 -seeds=127.0.0.1:8080
   ```

   ```bash
   # Terminale 3 (Nodo C)
   go run . -port=8082 -seeds=127.0.0.1:8080,127.0.0.1:8081
   ```

   ```bash
   # Terminale 4 (Nodo D)
   go run . -port=8083 -seeds=127.0.0.1:8080,127.0.0.1:8081
   ```
   
4. Per fermare un nodo, usa `Ctrl+C` nel terminale corrispondente.
5. Per testare crash improvvisi, puoi terminare il processo del nodo senza preavviso.
   