package kitchen

import (
	"context"
	"os"
	"os/exec"
	"strconv"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/logoraster"
)

// Documentos con atributos o logotipo (etapa 9B). La cola, el lease, los reintentos y las reglas
// de resultado incierto no cambian: solo cambia cómo el transporte traduce la IR.
//   - ESC/POS por red y, en Windows (etapa 9E), la cola de la impresora instalada en Windows en
//     modo RAW: los bytes de documents.EncodeESCPos (GS !, ESC E y GS v 0).
//   - CUPS: solo si la impresora está marcada «Esta impresora entiende ESC/POS (formato y logotipo)»
//     (desactivada por omisión) y además su cola admite datos raw (la detección de solo lectura de
//     cups_raw.go, consultada antes de enviar), los mismos bytes ESC/POS con lp -o raw. Una cola raw
//     no garantiza que la impresora entienda ESC/POS (una de oficina o una que solo admite raster con
//     su controlador imprimiría basura): sin la marca o sin raw se imprime el texto del documento por
//     el camino de siempre, sin negrita, tamaños ni logotipo, y el resultado lo dice, sin fallar.
//   - Una imagen cuyos bits no están disponibles, o que se convirtió para otro ancho de papel (un
//     trabajo redirigido, con respaldo o copiado en una impresora de otro ancho conserva su IR), se
//     omite con aviso, sin fallar la impresión.

const (
	noRawNotice     = " La cola no admite datos raw: el documento se envió sin formato ni logotipo."
	notESCPosNotice = " La impresora no está marcada como «Esta impresora entiende ESC/POS (formato y logotipo)»: " +
		"el documento se envió sin formato ni logotipo."
	missingLogoNote = " No se encontró la imagen del logotipo: se omitió."
	otherWidthNote  = " El logotipo se preparó para otro ancho de papel que el de esta impresora: se omitió."
)

// submitFormatted entrega un documento con atributos según el transporte de su impresora.
func (transport HostTransport) submitFormatted(ctx context.Context, job PrintJob) Outcome {
	printer := job.PrinterSnapshot
	cut := printer.PaperCut()
	rasters, note := printableRasters(job)
	switch printer.Transport {
	case "escpos_tcp":
		outcome := writeESCPos(ctx, job, documents.EncodeESCPos(job.Layout, rasters, cut))
		outcome.Message += note
		return outcome
	case "cups":
		// En Windows (etapa 9E) la cola del equipo recibe siempre ESC/POS en modo RAW: la marca
		// «Esta impresora entiende ESC/POS» no aplica y no hay camino de texto.
		if spooler, windows := transport.windowsSpooler(); windows {
			outcome := submitWindowsDocument(ctx, spooler, job, documents.EncodeESCPos(job.Layout, rasters, cut))
			outcome.Message += note
			return outcome
		}
		// Sin la marca explícita de la impresora no se consulta la cola: se imprime el texto.
		if !printer.AcceptsESCPos {
			outcome := transport.submitText(ctx, job)
			outcome.Message += notESCPosNotice
			return outcome
		}
		capability := inspectQueue(ctx, transport.runner(), printer.Destination)
		if !capability.Checked {
			return Outcome{State: "error", Code: capability.unavailableCode(),
				Message: "No se pudo consultar la cola CUPS del servidor; no se envió el documento."}
		}
		if !capability.Raw {
			outcome := transport.submitText(ctx, job)
			outcome.Message += noRawNotice
			return outcome
		}
		outcome := submitRawCUPS(ctx, job, documents.EncodeESCPos(job.Layout, rasters, cut), transport.runner())
		outcome.Message += note
		return outcome
	}
	return Outcome{State: "intervention", Code: ProblemUnsupported, Message: unsupportedPrinterMessage}
}

// printableRasters devuelve los bits de las imágenes que la impresora del trabajo puede imprimir y
// el aviso de las que se omiten. Una imagen se omite si se convirtió para otro ancho de papel (el
// layout se renderizó para la impresora original: un raster de 576 puntos en una impresora de 58 mm,
// de 384, daría un resultado indefinido en papel) o si sus bits no están disponibles.
func printableRasters(job PrintJob) (map[string]*documents.Raster, string) {
	result := map[string]*documents.Raster{}
	layout := job.Layout
	if layout == nil {
		return result, ""
	}
	printable, known := logoraster.PrintableDots(job.PrinterSnapshot.PaperWidth)
	otherWidth := layout.PaperWidth != 0 && layout.PaperWidth != job.PrinterSnapshot.PaperWidth
	omitted := map[string]bool{}
	for _, line := range layout.Lines {
		if line.Image == nil {
			continue
		}
		raster := job.Rasters[line.Image.RasterID]
		tooWide := raster != nil && known && raster.WidthDots > printable
		if otherWidth || tooWide {
			omitted[line.Image.RasterID] = true
			continue
		}
		if raster != nil {
			result[line.Image.RasterID] = raster
		}
	}
	note := ""
	if len(omitted) > 0 {
		note += otherWidthNote
	}
	for _, id := range documents.MissingRasters(layout, result) {
		if !omitted[id] {
			note += missingLogoNote
			break
		}
	}
	return result, note
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
