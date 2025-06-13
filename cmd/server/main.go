package main

import (
    "log"
    "net/http"
    "os"

    "github.com/joho/godotenv"
    "football-chatbot/internal/fetcher"
    "football-chatbot/internal/ws"
)

func main() {

    if err := godotenv.Load(); err != nil {
        log.Println("⚠️  .env no encontrado, usando variables de entorno del sistema")
    }

    logFile, err := os.OpenFile("bot.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err != nil {
        log.Fatalf("❌ No pude abrir bot.log: %v", err)
    }
    defer logFile.Close()

    fileLogger := log.New(logFile, "", log.LstdFlags)
    console := log.New(os.Stdout, "", log.LstdFlags)

    apiKey := os.Getenv("FOOTBALL_DATA_API_KEY")
    if apiKey == "" {
        console.Fatal("❌ FALLA: define FOOTBALL_DATA_API_KEY en .env o entorno")
    }
    schedule := os.Getenv("FETCH_SCHEDULE")
    if schedule == "" {
        schedule = "@every 1m"
    }

    console.Println("🔧 Iniciando servidor...")

    wsServer := ws.NewServer(console)               // pasamos console también para errores WS
    fc := fetcher.NewFetcher(apiKey, fileLogger)    // logger de detalle
    updates := make(chan []byte)
    fc.Subscribe(updates)

    console.Println("⏳ Ejecutando primer fetch…")
    fc.FetchOnce() // disparo inmediato

    // reenviar al WS
    go func() {
        for msg := range updates {
            wsServer.Broadcast(msg)
        }
    }()

    // arrancar cron
    if err := fc.Start(schedule); err != nil {
        console.Fatalf("❌ No pude iniciar el cron: %v", err)
    }

    fs := http.FileServer(http.Dir("public"))
    http.Handle("/", fs)
    http.HandleFunc("/ws", wsServer.HandleWS)

    console.Println("🚀 Servidor escuchando en http://localhost:8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        console.Fatalf("❌ ListenAndServe: %v", err)
    }
}
