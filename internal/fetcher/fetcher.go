package fetcher

import (
    "fmt"
    "log"
    "time"

    "github.com/go-resty/resty/v2"
    "github.com/robfig/cron/v3"
)

// Fetcher consulta periódicamente la API y usa un logger detallado.
type Fetcher struct {
    client      *resty.Client
    cron        *cron.Cron
    subscribers []chan []byte
    logger      *log.Logger
}

// NewFetcher recibe tu API key y un *log.Logger para detalles
func NewFetcher(apiKey string, logger *log.Logger) *Fetcher {
    client := resty.New().
        SetHostURL("https://api.football-data.org/v4").
        SetHeader("X-Auth-Token", apiKey)

    return &Fetcher{
        client:      client,
        cron:        cron.New(),
        subscribers: nil,
        logger:      logger,
    }
}

func (f *Fetcher) Subscribe(ch chan []byte) {
    f.subscribers = append(f.subscribers, ch)
}

func (f *Fetcher) Start(schedule string) error {
    // añade tarea periódica
    if _, err := f.cron.AddFunc(schedule, f.fetch); err != nil {
        return err
    }
    f.cron.Start()
    return nil
}

// FetchOnce para disparo manual inmediato
func (f *Fetcher) FetchOnce() {
    f.fetch()
}

func (f *Fetcher) fetch() {
    // detalles al archivo via f.logger
    f.logger.Println("🔍 Haciendo fetch a la API…")

    // rango: primer día de mes — hoy
    now := time.Now()
    y, m, _ := now.Date()
    loc := now.Location()
    first := time.Date(y, m, 1, 0, 0, 0, 0, loc)
    from := first.Format("2006-01-02")
    to   := now.Format("2006-01-02")

    endpoint := fmt.Sprintf("/matches?dateFrom=%s&dateTo=%s", from, to)
    f.logger.Printf("🔗 GET %s …", endpoint)

    resp, err := f.client.R().Get(endpoint)
    if err != nil {
        f.logger.Printf("❌ Error HTTP: %v", err)
        return
    }
    f.logger.Printf("📬 Estado: %s", resp.Status())

    body := resp.Body()
    if len(body) > 200 {
        f.logger.Printf("📄 Body (preview): %s…", string(body[:200]))
    } else {
        f.logger.Printf("📄 Body: %s", string(body))
    }

    // reenvío
    for _, ch := range f.subscribers {
        select {
			case ch <- body:
			default:
        }
    }
}
