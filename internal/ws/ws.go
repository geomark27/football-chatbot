package ws

import (
    "encoding/json"
    "log"
    "net/http"
    "sync"

    "github.com/gorilla/websocket"
)

// Filters almacena los criterios de filtrado por cliente.
type Filters struct {
    Leagues []string `json:"leagues"`
    Status  []string `json:"status"`
}

// Server gestiona conexiones WebSocket, filtros por cliente y difunde mensajes.
type Server struct {
    upgrader websocket.Upgrader
    clients  map[*websocket.Conn]Filters
    mu       sync.Mutex
    console  *log.Logger
}

// NewServer inicializa un servidor WS con CORS permisivo y logger de consola.
func NewServer(consoleLogger *log.Logger) *Server {
    return &Server{
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool { return true },
        },
        clients: make(map[*websocket.Conn]Filters),
        console: consoleLogger,
    }
}

// HandleWS maneja la rutina de handshake, registro de cliente y lectura de mensajes.
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        s.console.Printf("❌ Error al abrir WebSocket: %v", err)
        http.Error(w, "No se pudo abrir WebSocket", http.StatusBadRequest)
        return
    }

    // Registro inicial con filtros vacíos (todos)
    s.mu.Lock()
    s.clients[conn] = Filters{}
    s.mu.Unlock()
    s.console.Println("✔️ Cliente conectado")

    // Rutina para procesar mensajes entrantes de este cliente
    go s.readPump(conn)
}

// readPump lee mensajes del cliente para actualizar filtros o procesar comandos.
func (s *Server) readPump(conn *websocket.Conn) {
    defer func() {
        s.mu.Lock()
        delete(s.clients, conn)
        s.mu.Unlock()
        conn.Close()
        s.console.Println("ℹ️ Cliente desconectado")
    }()

    for {
        _, msg, err := conn.ReadMessage()
        if err != nil {
            return // desconexión u error
        }
        var cmd struct {
            Action  string   `json:"action"`
            Leagues []string `json:"leagues"`
            Status  []string `json:"status"`
        }
        if err := json.Unmarshal(msg, &cmd); err != nil {
            s.console.Printf("⚠️ Mensaje inválido de cliente: %v", err)
            continue
        }
        if cmd.Action == "updateFilters" {
            s.mu.Lock()
            s.clients[conn] = Filters{Leagues: cmd.Leagues, Status: cmd.Status}
            s.mu.Unlock()
            s.console.Printf("🔄 Filtros actualizados: ligas=%v estados=%v", cmd.Leagues, cmd.Status)
        }
    }
}

// Broadcast envía un mensaje JSON a todos los clientes conectados aplicando sus filtros.
func (s *Server) Broadcast(data []byte) {
    // Parseamos la carga para filtrar matches
    var payload struct {
        Matches []struct {
            Competition struct {
                Code string `json:"code"`
            } `json:"competition"`
            Status string `json:"status"`
        } `json:"matches"`
    }
    if err := json.Unmarshal(data, &payload); err != nil {
        s.console.Printf("⚠️ No se pudo parsear payload: %v", err)
        return
    }

    s.mu.Lock()
    defer s.mu.Unlock()

    // Para cada cliente, aplica filtros y envía si corresponde
    for conn, filt := range s.clients {
        // Filtrar matches
        var filtered []interface{}
        var raw map[string]interface{}
        json.Unmarshal(data, &raw)
        for _, m := range payload.Matches {
            okLeague := len(filt.Leagues) == 0
            for _, lg := range filt.Leagues {
                if lg == m.Competition.Code {
                    okLeague = true
                    break
                }
            }
            okStatus := len(filt.Status) == 0
            for _, st := range filt.Status {
                if st == m.Status {
                    okStatus = true
                    break
                }
            }
            if okLeague && okStatus {
                // Extraemos el match original para mantener estructura
                for _, orig := range raw["matches"].([]interface{}) {
                    item := orig.(map[string]interface{})
                    comp := item["competition"].(map[string]interface{})
                    if comp["code"] == m.Competition.Code && item["status"] == m.Status {
                        filtered = append(filtered, item)
                        break
                    }
                }
            }
        }
        // Solo enviar si hay elementos.
        if len(filtered) > 0 {
            raw["matches"] = filtered
            if msg, err := json.Marshal(raw); err == nil {
                if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
                    s.console.Printf("❌ Error al enviar mensaje filtrado: %v", err)
                    conn.Close()
                    delete(s.clients, conn)
                }
            }
        }
    }
}
