package administration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (service *Service) eventSnapshot(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if err := principal.require("events.read"); err != nil {
		return nil, err
	}
	for key := range request.Query {
		if key != "after" && key != "station_id" {
			return nil, invalid("Parámetro de eventos no permitido.")
		}
	}
	after := int64(0)
	if text := request.Query.Get("after"); text != "" {
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil || parsed < 0 {
			return nil, invalid("El cursor de eventos no es válido.")
		}
		after = parsed
	}
	stationID := request.Query.Get("station_id")
	if stationID != "" {
		if !identifierPattern.MatchString(stationID) {
			return nil, invalid("La estación no es válida.")
		}
		if !slices.Contains(principal.Permissions, "kitchen.stations.all") {
			var allowed bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.kitchen_device_stations WHERE device_id=$1 AND station_id=$2)`, principal.DeviceID, stationID).Scan(&allowed); err != nil {
				return nil, err
			}
			if !allowed {
				return nil, denied()
			}
		}
	}
	return events.Read(ctx, tx, after, principal.Permissions, principal.DeviceID, stationID)
}

func (service *Service) serveEvents(writer http.ResponseWriter, request *http.Request, local bool) {
	arguments := Request{Method: "GET", Path: "/api/v1/events/snapshot", Query: request.URL.Query(),
		DeviceID: request.Header.Get("X-Device-ID"), DeviceToken: request.Header.Get("X-Device-Token"),
		SessionToken: strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "), Local: local}
	if arguments.Query.Get("after") == "" && request.Header.Get("Last-Event-ID") != "" {
		arguments.Query.Set("after", request.Header.Get("Last-Event-ID"))
	}
	read := func() (*events.Batch, error) {
		ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
		defer cancel()
		result, err := service.Execute(ctx, arguments)
		if err != nil {
			return nil, err
		}
		return result.(*events.Batch), nil
	}
	initial, err := read()
	if err != nil {
		writeFault(writer, request, publicFault(err))
		return
	}
	service.streamMutex.Lock()
	if service.streams == nil {
		service.streams = map[string]int{}
	}
	if service.streams[arguments.DeviceID] >= 2 {
		service.streamMutex.Unlock()
		writeFault(writer, request, &Fault{Status: 429, Code: "EVENT_CONNECTION_LIMIT", Message: "La terminal ya tiene dos conexiones de eventos."})
		return
	}
	service.streams[arguments.DeviceID]++
	service.streamMutex.Unlock()
	defer func() {
		service.streamMutex.Lock()
		service.streams[arguments.DeviceID]--
		service.streamMutex.Unlock()
	}()
	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Accel-Buffering", "no")
	control := http.NewResponseController(writer)
	send := func(name string, value any, cursor int64) error {
		_ = control.SetWriteDeadline(time.Now().Add(10 * time.Second))
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", cursor, name, encoded); err != nil {
			return err
		}
		return control.Flush()
	}
	if err := send("changes", initial, initial.Cursor); err != nil {
		return
	}
	arguments.Query.Set("after", strconv.FormatInt(initial.Cursor, 10))
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	lifetime := time.NewTimer(30 * time.Second)
	defer lifetime.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-lifetime.C:
			return
		case <-ticker.C:
			// Se revalidan sesión, dispositivo, permiso y estación en cada lectura.
			batch, err := read()
			if err != nil {
				problem := publicFault(err)
				_ = send("interrupted", map[string]any{"error": problem.Code, "status": problem.Status}, 0)
				return
			}
			if err := send("changes", batch, batch.Cursor); err != nil {
				return
			}
			arguments.Query.Set("after", strconv.FormatInt(batch.Cursor, 10))
		}
	}
}
