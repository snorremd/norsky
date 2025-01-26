package firehose

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

// Add Prometheus metrics
var (
	wsConnectionAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "norsky_jetstream_connection_attempts_total",
		Help: "The total number of connection attempts to the Jetstream websocket",
	})

	wsConnectionErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "norsky_jetstream_connection_errors_total",
		Help: "The total number of connection errors encountered",
	})

	wsCurrentConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "norsky_jetstream_current_connections",
		Help: "The current number of active Jetstream websocket connections",
	})

	wsConnectionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "norsky_jetstream_connection_duration_seconds",
		Help:    "Duration of Jetstream websocket connections",
		Buckets: prometheus.ExponentialBuckets(1, 2, 10), // Start at 1s, double each bucket, 10 buckets
	})

	wsPingLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "norsky_jetstream_ping_latency_seconds",
		Help:    "Latency of websocket ping/pong round trips",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // Start at 1ms, double each bucket, 10 buckets
	})

	wsHostSwitches = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "norsky_jetstream_host_switches_total",
		Help: "Number of times the connection switched to a different host",
	}, []string{"from_host", "to_host"})
)

const (
	wsReadBufferSize  = 1024 * 1024 // 1MB
	wsWriteBufferSize = 1024        // 1KB
	wsReadTimeout     = 60 * time.Second
	wsWriteTimeout    = 10 * time.Second
	wsPingInterval    = 30 * time.Second
)

// JetstreamConfig holds configuration for the Jetstream connection
type JetstreamConfig struct {
	// Hosts is a list of Jetstream endpoints to try in order
	// e.g. ["wss://jetstream1.us-east.bsky.network", "wss://jetstream2.us-east.bsky.network"]
	Hosts             []string
	WantedCollections []string
	WantedDids        []string
	Cursor            int64
	Compress          bool
	RequireHello      bool
	UserAgent         string
}

// RawMessage represents an unparsed message from the websocket
type RawMessage struct {
	MessageType int    // websocket.TextMessage or websocket.BinaryMessage
	Data        []byte // Raw message data
}

// SubscribeJetstream establishes and maintains a websocket connection to the Jetstream service
func SubscribeJetstream(ctx context.Context, config JetstreamConfig) (*websocket.Conn, error) {

	log.WithFields(log.Fields{
		"hosts": config.Hosts,
	}).Info("Subscribing to Jetstream")

	if len(config.Hosts) == 0 {
		return nil, fmt.Errorf("no hosts provided in config")
	}

	currentHostIdx := 0

	// Configure websocket dialer
	dialer := websocket.Dialer{
		ReadBufferSize:   wsReadBufferSize,
		WriteBufferSize:  wsWriteBufferSize,
		HandshakeTimeout: 45 * time.Second,
		NetDialContext: (&net.Dialer{
			Timeout:   45 * time.Second,
			KeepAlive: 45 * time.Second,
		}).DialContext,
	}

	// Set up exponential backoff for reconnection attempts
	backoff := backoff.NewExponentialBackOff()
	backoff.InitialInterval = 100 * time.Millisecond
	backoff.MaxInterval = 30 * time.Second
	backoff.Multiplier = 1.5
	backoff.MaxElapsedTime = 0 // Never stop retrying

	var conn *websocket.Conn

	// Connection loop with retry and failover logic
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			currentHost := config.Hosts[currentHostIdx]

			// Build URL with query parameters
			u, err := url.Parse(fmt.Sprintf("%s/subscribe", currentHost))
			if err != nil {
				return nil, fmt.Errorf("failed to parse URL: %w", err)
			}

			q := u.Query()
			if len(config.WantedCollections) > 0 {
				for _, collection := range config.WantedCollections {
					q.Add("wantedCollections", collection)
				}
			}
			if len(config.WantedDids) > 0 {
				for _, did := range config.WantedDids {
					q.Add("wantedDids", did)
				}
			}
			if config.Cursor != 0 {
				q.Set("cursor", fmt.Sprintf("%d", config.Cursor))
			}
			if config.Compress {
				q.Set("compress", "true")
			}
			if config.RequireHello {
				q.Set("requireHello", "true")
			}
			u.RawQuery = q.Encode()

			// Set up headers
			headers := http.Header{}
			if config.UserAgent != "" {
				headers.Set("User-Agent", config.UserAgent)
			}

			if config.Compress {
				headers.Set("Accept-Encoding", "zstd")
			}

			wsConnectionAttempts.Inc()

			var dialErr error
			conn, _, dialErr = dialer.Dial(u.String(), headers)

			if dialErr != nil {
				wsConnectionErrors.Inc()
				log.Errorf("Error connecting to Jetstream host %s: %s", currentHost, dialErr)

				// Try next host
				nextHostIdx := (currentHostIdx + 1) % len(config.Hosts)
				if nextHostIdx != currentHostIdx {
					wsHostSwitches.WithLabelValues(currentHost, config.Hosts[nextHostIdx]).Inc()
					log.Infof("Switching from host %s to %s", currentHost, config.Hosts[nextHostIdx])
					currentHostIdx = nextHostIdx
					// Reset backoff when switching hosts
					backoff.Reset()
					continue
				}

				// If we've tried all hosts, wait before retrying
				time.Sleep(backoff.NextBackOff())
				continue
			}

			// Reset backoff on successful connection
			backoff.Reset()
			wsCurrentConnections.Inc()

			// Start connection duration timer
			connStart := time.Now()
			go func() {
				<-ctx.Done()
				wsConnectionDuration.Observe(time.Since(connStart).Seconds())
				wsCurrentConnections.Dec()
			}()

			// Set up connection handlers
			setupConnectionHandlers(conn)

			// Start ping routine
			go managePingPong(ctx, conn)

			return conn, nil
		}
	}
}

// setupConnectionHandlers configures the websocket connection handlers
func setupConnectionHandlers(conn *websocket.Conn) {
	// Set initial deadlines
	conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))

	// Add connection close handler
	conn.SetCloseHandler(func(code int, text string) error {
		log.Infof("WebSocket connection closed with code %d: %s", code, text)
		return nil
	})

	// Set ping handler
	conn.SetPingHandler(func(appData string) error {
		log.Debug("Received ping from server")
		return conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	})

	// Set pong handler
	conn.SetPongHandler(func(appData string) error {
		log.Debug("Received pong from server")
		return conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	})
}

// managePingPong handles the ping/pong keepalive for the websocket connection
func managePingPong(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingStart := time.Now()
			log.Debug("Sending ping to check connection")

			if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(wsWriteTimeout)); err != nil {
				log.Warn("Ping failed, closing connection for restart: ", err)
				wsConnectionErrors.Inc()
				conn.Close()
				return
			}

			// Measure ping latency when we receive the pong
			conn.SetPongHandler(func(appData string) error {
				wsPingLatency.Observe(time.Since(pingStart).Seconds())
				return conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
			})

			// Reset read deadline after successful ping
			if err := conn.SetReadDeadline(time.Now().Add(wsReadTimeout)); err != nil {
				log.Warn("Failed to set read deadline, closing connection: ", err)
				wsConnectionErrors.Inc()
				conn.Close()
				return
			}
		}
	}
}

// SubscribeJetstreamWithMessages establishes a websocket connection and returns a channel of raw messages
func SubscribeJetstreamWithMessages(ctx context.Context, config JetstreamConfig, workerQueue chan *RawMessage) error {
	log.Infof("Subscribing to Jetstream with messages")
	conn, err := SubscribeJetstream(ctx, config)
	if err != nil {
		return err
	}

	// Start message reading goroutine
	go func() {
		defer conn.Close()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				messageType, message, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Errorf("Unexpected websocket close: %v", err)
					}
					wsConnectionErrors.Inc()
					return
				}

				rawMsg := &RawMessage{
					MessageType: messageType,
					Data:        message,
				}

				workerQueue <- rawMsg
			}
		}
	}()

	return nil
}
