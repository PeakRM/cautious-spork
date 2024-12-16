package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Trade struct {
	ID        uint    `gorm:"primaryKey" json:"id"`
	Price     float64 `json:"p,string"`
	Quantity  float64 `json:"q,string"`
	Timestamp int64   `json:"T"`
	IsBuyer   bool    `json:"m"`
}

type DollarImbalanceBar struct {
	ID               uint    `gorm:"primaryKey"`
	Timestamp        int64   `json:"timestamp"`
	DollarImbalance  float64 `json:"dollar_imbalance"`
	ThresholdReached bool    `json:"threshold_reached"`
}

var (
	db        *gorm.DB
	tradeChan = make(chan Trade, 1000)
	barChan   = make(chan DollarImbalanceBar, 100)
	mu        sync.Mutex
)

// Initialize PostgreSQL connection
func initDB() {
	var err error
	dsn := "host=localhost user=postgres password=yourpassword dbname=trades port=5432 sslmode=disable"
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	db.AutoMigrate(&Trade{}, &DollarImbalanceBar{})
}

// Stream trades from Binance WebSocket API
func streamTrades() {
	url := "wss://stream.binance.com:9443/ws/btcusdt@trade"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("WebSocket connection error:", err)
	}
	defer conn.Close()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("WebSocket read error:", err)
			return
		}

		var trade Trade
		if err := json.Unmarshal(message, &trade); err != nil {
			log.Println("JSON unmarshal error:", err)
			continue
		}
		tradeChan <- trade
	}
}

// Process and store trades, calculate dollar imbalance bars
func processTrades() {
	var (
		dollarImbalance float64
		buyVolume       float64
		sellVolume      float64
		threshold       = 10000.0 // Example threshold for dollar imbalance
	)

	for trade := range tradeChan {
		db.Create(&trade)

		// Calculate dollar imbalance
		dollarValue := trade.Price * trade.Quantity
		if trade.IsBuyer {
			buyVolume += dollarValue
		} else {
			sellVolume += dollarValue
		}
		dollarImbalance = math.Abs(buyVolume - sellVolume)

		// Generate a bar if the imbalance exceeds the threshold
		if dollarImbalance >= threshold {
			bar := DollarImbalanceBar{
				Timestamp:        trade.Timestamp,
				DollarImbalance:  dollarImbalance,
				ThresholdReached: true,
			}
			db.Create(&bar)
			barChan <- bar

			// Reset volumes
			buyVolume, sellVolume = 0, 0
		}
	}
}

// Dashboard server
func startDashboard() {
	r := gin.Default()

	r.GET("/bars", func(c *gin.Context) {
		var bars []DollarImbalanceBar
		db.Order("timestamp desc").Limit(50).Find(&bars)
		c.JSON(http.StatusOK, bars)
	})

	r.GET("/", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Dollar Imbalance Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
</head>
<body>
    <h1>Dollar Imbalance Bars</h1>
    <canvas id="chart" width="800" height="400"></canvas>
    <script>
        async function fetchBars() {
            const response = await fetch('/bars');
            return response.json();
        }

        async function renderChart() {
            const bars = await fetchBars();
            const labels = bars.map(bar => new Date(bar.timestamp).toLocaleTimeString());
            const data = bars.map(bar => bar.dollar_imbalance);

            new Chart(document.getElementById('chart').getContext('2d'), {
                type: 'bar',
                data: {
                    labels: labels,
                    datasets: [{
                        label: 'Dollar Imbalance',
                        data: data,
                        backgroundColor: 'rgba(75, 192, 192, 0.2)',
                        borderColor: 'rgba(75, 192, 192, 1)',
                        borderWidth: 1
                    }]
                },
                options: {
                    scales: {
                        y: { beginAtZero: true }
                    }
                }
            });
        }

        renderChart();
        setInterval(renderChart, 5000); // Refresh every 5 seconds
    </script>
</body>
</html>
		`)
	})

	r.Run(":8080")
}

func main() {
	initDB()

	go streamTrades()
	go processTrades()
	startDashboard()
}