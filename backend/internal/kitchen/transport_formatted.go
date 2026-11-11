package kitchen

import (
	"context"
	"os"
	"os/exec"
	"strconv"

	"local/ad-el-pos/server/internal/documents"
)

// Documentos con atributos o logotipo (etapa 9B). La cola, el lease, los reintentos y las reglas
// de resultado incierto no cambian: solo cambia cómo el transporte traduce la IR.
//   - ESC/POS por red: los bytes de documents.EncodeESCPos (GS !, ESC E y GS v 0).
//   - CUPS: si la cola admite datos raw (la detección de solo lectura de cups_raw.go, consultada
//     antes de enviar), los mismos bytes ESC/POS con lp -o raw; si no, el texto del documento por
//     el camino de siempre, sin negrita, tamaños ni logotipo, y el resultado lo dice, sin fallar.
//   - Una imagen cuyos bits no están disponibles se omite con aviso, sin fallar la impresión.

const (
	noRawNotice     = " La cola no admite datos raw: el documento se envió sin formato ni logotipo."
	missingLogoNote = " No se encontró la imagen del logotipo: se omitió."
)

// submitFormatted entrega un documento con atributos según el transporte de su impresora.
func (transport HostTransport) submitFormatted(ctx context.Context, job PrintJob) Outcome {
	printer := job.PrinterSnapshot
	cut := printer.Cut && printer.SupportsCut
	note := ""
	if len(documents.MissingRasters(job.Layout, job.Rasters)) > 0 {
		note = missingLogoNote
	}
	switch printer.Transport {
	case "escpos_tcp":
		outcome := writeESCPos(ctx, job, documents.EncodeESCPos(job.Layout, job.Rasters, cut))
		outcome.Message += note
		return outcome
	case "cups":
		capability := inspectQueue(ctx, transport.runner(), printer.Destination)
		if !capability.Checked {
			return Outcome{State: "error", Message: "No se pudo consultar la cola CUPS del servidor; no se envió el documento."}
		}
		if !capability.Raw {
			outcome := transport.submitText(ctx, job)
			outcome.Message += noRawNotice
			return outcome
		}
		outcome := submitRawCUPS(ctx, job, documents.EncodeESCPos(job.Layout, job.Rasters, cut), transport.runner())
		outcome.Message += note
		return outcome
	}
	return Outcome{State: "intervention", Message: "El destino requiere un agente de impresión futuro."}
}

// submitText imprime el texto del documento por el camino de CUPS anterior a la 9B.
func (transport HostTransport) submitText(ctx context.Context, job PrintJob) Outcome {
	if transport.textCUPS != nil {
		return transport.textCUPS(ctx, job)
	}
	return submitCUPS(ctx, job)
}

// rawDocumentArguments son los argumentos separados de lp para un documento ESC/POS en modo raw:
// la cola, raw (CUPS no filtra los bytes), las copias de la impresora y el título técnico.
func rawDocumentArguments(queue string, copies int, jobID, filename string) []string {
	return []string{"-d", queue, "-o", "raw", "-n", strconv.Itoa(copies), "-t", "ADEL-" + jobID, filename}
}

// submitRawCUPS envía los bytes ESC/POS de un documento a una cola CUPS que admite datos raw, desde
// un archivo temporal privado que se elimina al terminar. Una vez iniciado lp, un resultado sin
// identificador de trabajo es incierto y nunca se reintenta solo.
func submitRawCUPS(ctx context.Context, job PrintJob, data []byte, commands commandRunner) Outcome {
	queue := job.PrinterSnapshot.Destination
	file, err := os.CreateTemp("", "adel-print-*.bin")
	if err != nil {
		return Outcome{State: "error", Message: "No se pudo preparar el archivo privado; no se envió el documento."}
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return Outcome{State: "error", Message: "No se pudo escribir el documento local."}
	}
	if err := file.Close(); err != nil {
		return Outcome{State: "error", Message: "No se pudo finalizar el documento local."}
	}
	output, err := commands.run(ctx, true, "/usr/bin/lp", rawDocumentArguments(queue, job.PrinterSnapshot.Copies, job.ID, file.Name())...)
	reference := cupsJobReference(queue, output)
	if err == nil && reference != "" {
		return Outcome{State: "submitted", Reference: reference, Accepted: true, Message: "Aceptado por CUPS; no confirma salida física de papel."}
	}
	if _, notStarted := err.(*exec.Error); notStarted {
		return Outcome{State: "error", Message: "No se pudo ejecutar el transporte CUPS; no se envió el documento."}
	}
	return Outcome{State: "uncertain", Message: "CUPS no devolvió un resultado inequívoco. Revisar su cola antes de autorizar otra copia."}
}
