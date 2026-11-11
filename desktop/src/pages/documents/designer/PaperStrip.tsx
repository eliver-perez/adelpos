import type { CSSProperties } from 'react';
import type { DocumentPreview, PreviewLine } from '../../../services/documents';

// Tira de papel térmico con las líneas exactas de la IR de Go: texto monoespaciado ya cortado,
// alineado y saneado por Go, con el ancho de columnas real del papel (30 o 42; 15 o 21 con texto
// grande de Cocina). La interfaz no corta ni rellena nada: solo muestra cada línea y su atributo
// (negrita, doble alto, doble ancho, doble) y el logotipo con la imagen convertida por Go. Las
// líneas son elementos en línea con display: block para poder vivir dentro de un botón.
//
// Las líneas consecutivas de un mismo bloque forman un segmento que se puede tocar para elegir ese
// bloque en la estructura; el segmento del bloque elegido se resalta.

interface Segment {
  blockID: string;
  start: number;
  lines: PreviewLine[];
}

function segmentsOf(lines: PreviewLine[]) {
  const segments: Segment[] = [];
  lines.forEach((line, index) => {
    const last = segments[segments.length - 1];
    if (last && last.blockID === line.block_id) {
      last.lines.push(line);
      return;
    }
    segments.push({ blockID: line.block_id, start: index, lines: [line] });
  });
  return segments;
}

function PaperLine({ line }: { line: PreviewLine }) {
  if (line.image) {
    const image = line.image;
    if (!image.data_uri) {
      return (
        <span className="paper-line paper-image-missing">
          [Logotipo · {image.width_dots} × {image.height_dots} puntos]
        </span>
      );
    }
    return (
      <span className="paper-line paper-image">
        <img
          src={image.data_uri}
          alt="Logotipo del restaurante tal como se imprime"
          width={image.width_dots}
          height={image.height_dots}
        />
      </span>
    );
  }
  return (
    <span className={`paper-line ${line.bold ? 'paper-bold' : ''}`} data-size={line.size}>
      <span className="paper-text">{line.text === '' ? ' ' : line.text}</span>
    </span>
  );
}

export function PaperStrip({
  preview,
  selectedID = null,
  blockName,
  onSelect,
  label,
}: {
  preview: DocumentPreview;
  selectedID?: string | null;
  // Nombre del bloque para el texto accesible de cada segmento.
  blockName?: (blockID: string) => string;
  onSelect?: (blockID: string) => void;
  label: string;
}) {
  // Ancho imprimible en caracteres normales: con el texto grande de Cocina, Go entrega la mitad de
  // columnas y cada carácter ocupa el doble, así que la tira conserva el ancho del papel.
  const printable = preview.text_size === 'large' ? preview.columns * 2 : preview.columns;
  const style = { '--paper-print': printable } as CSSProperties;
  const segments = segmentsOf(preview.lines);
  return (
    <div
      className="paper-strip"
      data-paper={preview.paper_width}
      data-text-size={preview.text_size}
      style={style}
      role="figure"
      aria-label={label}
    >
      <div className="paper-sheet">
        {segments.map((segment) => {
          const content = segment.lines.map((line, offset) => (
            <PaperLine key={segment.start + offset} line={line} />
          ));
          if (!onSelect || !segment.blockID) {
            return (
              <span key={segment.start} className="paper-segment">
                {content}
              </span>
            );
          }
          const selected = segment.blockID === selectedID;
          return (
            <button
              type="button"
              key={segment.start}
              className={`paper-segment paper-segment-button ${selected ? 'paper-segment-selected' : ''}`}
              aria-pressed={selected}
              aria-label={`Elegir ${blockName?.(segment.blockID) ?? 'el bloque'}`}
              data-block-id={segment.blockID}
              onClick={() => onSelect(segment.blockID)}
            >
              {content}
            </button>
          );
        })}
        {preview.lines.length === 0 && (
          <p className="paper-empty">Esta plantilla no imprime ninguna línea con estos datos.</p>
        )}
      </div>
    </div>
  );
}
