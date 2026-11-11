package kitchen

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/documents"
)

// Comandas reales para la vista previa del diseñador (etapa 9B). Solo leen y aplican las reglas
// de consulta de la cola de cocina: kitchen.tickets.read, solo las estaciones de la terminal (o
// todas con kitchen.stations.all) y los importes solo con catalog.prices.read.

// PreviewSource devuelve los datos de una comanda o aviso emitido, con las secciones que el actor
// puede ver.
func PreviewSource(ctx context.Context, tx pgx.Tx, actor Actor, ticketID string) (*documents.KitchenData, documents.Kind, error) {
	if err := actor.Require("kitchen.tickets.read"); err != nil {
		return nil, "", err
	}
	if err := validID(ticketID); err != nil {
		return nil, "", err
	}
	var kind string
	if err := tx.QueryRow(ctx, "SELECT kind FROM app.kitchen_tickets WHERE id=$1", ticketID).Scan(&kind); err != nil {
		return nil, "", err
	}
	if kind != "normal" && kind != "modification" && kind != "cancellation" {
		return nil, "", invalid("El documento no es una comanda ni un aviso de cocina.")
	}
	handler := service{ctx: ctx, tx: tx, request: Request{Actor: actor}}
	document, err := handler.filteredDocument(ticketID)
	if err != nil {
		return nil, "", err
	}
	data := DocumentData(*document)
	return &data, documentKind(kind), nil
}

// RecentDocuments lista las comandas o avisos recientes de un tipo de ticket (normal, modification
// o cancellation) de las estaciones que el actor puede consultar.
func RecentDocuments(ctx context.Context, tx pgx.Tx, actor Actor, ticketKind string) ([]documents.RecentDocument, error) {
	if err := actor.Require("kitchen.tickets.read"); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id,t.created_at,coalesce((t.document->'context'->>'folio')::bigint,0),
        coalesce(t.document->'context'->>'account_name','') FROM app.kitchen_tickets t
        WHERE t.kind=$1 AND ($2 OR EXISTS(SELECT FROM app.kitchen_device_stations d WHERE d.device_id=$3 AND d.station_id=ANY(t.station_ids)))
        ORDER BY t.created_at DESC,t.id DESC LIMIT 20`, ticketKind, actor.Can("kitchen.stations.all"), actor.DeviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	kind := documentKind(ticketKind)
	result := []documents.RecentDocument{}
	for rows.Next() {
		var item documents.RecentDocument
		var folio int64
		var account string
		var created time.Time
		if err := rows.Scan(&item.TicketID, &created, &folio, &account); err != nil {
			return nil, err
		}
		item.Kind = kind
		item.CreatedAt = created
		item.Label = "Cuenta #" + strconv.FormatInt(folio, 10) + " · " + account
		result = append(result, item)
	}
	return result, rows.Err()
}

// SampleSettings devuelve la configuración vigente de Cocina con la forma que copia cada comanda:
// la vista previa con datos de muestra y la muestra impresa la aplican con la misma regla que una
// comanda real (documents.KitchenData.UseSettings), para que anticipen el papel.
func SampleSettings(ctx context.Context, tx pgx.Tx) (documents.KitchenSettings, error) {
	settings, err := one[Settings](ctx, tx, "SELECT to_jsonb(s) FROM app.kitchen_settings s")
	if err != nil {
		return documents.KitchenSettings{}, err
	}
	return documents.KitchenSettings{
		ShowPrices:        settings.ShowPrices,
		ShowNotes:         settings.ShowNotes,
		ShowEstablishment: settings.ShowEstablishment,
		TextSize:          settings.TextSize,
		LineSpacing:       settings.LineSpacing,
		QuantityStyle:     settings.QuantityStyle,
	}, nil
}

// ActivePrinters devuelve las impresoras activas para elegir dónde imprimir una muestra.
func ActivePrinters(ctx context.Context, tx pgx.Tx) ([]Printer, error) {
	return many[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE active ORDER BY name")
}

// ReadPrinter devuelve una impresora configurada.
func ReadPrinter(ctx context.Context, tx pgx.Tx, id string) (*Printer, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	return one[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", id)
}
