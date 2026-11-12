import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { SegmentedControl } from '../../components/SegmentedControl';
import {
  Check,
  IntegerField,
  ReferenceField,
  TextField,
} from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import {
  printerColumnChoices,
  printerColumns,
  type ExecuteKitchen,
  type Printer,
  type WidthTestResult,
} from '../../services/kitchen';
import { notifySuccess } from '../../services/notify';
import { Select } from '../../components/Select';

const emptyPrinter = (): Printer => ({
  id: '',
  name: '',
  transport: 'cups',
  destination: '',
  port: 9100,
  paper_width: 80,
  copies: 1,
  supports_cut: false,
  cut: false,
  cutter_option: '',
  accepts_escpos: false,
  columns: null,
  active: true,
  fallback_id: null,
  revision: 0,
});

export function PrintersAdmin({
  can,
  execute,
  busy,
}: {
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
}) {
  const printers = useLiveResource<Printer[]>('kitchen_printers');
  const discovery = useLiveResource<{ queue: string; status: string; options: string }[]>(
    'kitchen_discovery',
    { interval: 60000 },
  );
  const [value, setValue] = useState<Printer | null>(null);
  const current = printers.data?.find((entry) => entry.id === value?.id);
  const stale = !!value?.id && current?.revision !== value.revision;
  return (
    <div className="kitchen-editor-layout">
      <section className="panel form-stack">
        <div className="section-heading">
          <h2>Impresoras del servidor</h2>
          <button className="primary-button" onClick={() => setValue(emptyPrinter())}>
            Registrar impresora
          </button>
        </div>
        <Notice>{printers.error}</Notice>
        <p className="muted">
          Se utilizan las colas y conexiones disponibles en el host de Go. Una impresora conectada a
          otra terminal requiere un destino de red accesible o un agente futuro.
        </p>
        {printers.data?.map((printer) => (
          <button
            className="kitchen-list-button"
            key={printer.id}
            onClick={() => setValue(printer)}
          >
            <strong>{printer.name}</strong>
            <span>
              {printer.transport === 'cups'
                ? 'CUPS del servidor'
                : printer.transport === 'escpos_tcp'
                  ? 'ESC/POS por red'
                  : 'Agente futuro'}{' '}
              · {printer.paper_width} mm · {printerColumns(printer)} columnas ·{' '}
              {printer.active ? 'Activa' : 'Inactiva'}
            </span>
          </button>
        ))}
        <Accordion
          title="Colas anunciadas por CUPS"
          summary="La disponibilidad del spooler no confirma papel ni conexión física"
        >
          <button className="secondary-button" onClick={() => void discovery.reload()}>
            Consultar impresoras del servidor
          </button>
          <Notice>{discovery.error}</Notice>
          {discovery.data?.map((printer) => (
            <div className="kitchen-discovery" key={printer.queue}>
              <strong>{printer.queue}</strong>
              <p className="muted text-sm">{printer.status}</p>
              <button
                className="secondary-button"
                onClick={() =>
                  setValue({ ...emptyPrinter(), name: printer.queue, destination: printer.queue })
                }
              >
                Usar esta cola
              </button>
              <Accordion title={`Opciones del controlador ${printer.queue}`}>
                <pre className="kitchen-document">
                  {printer.options || 'El controlador no anunció opciones adicionales.'}
                </pre>
              </Accordion>
            </div>
          ))}
          {discovery.data?.length === 0 && (
            <p className="muted">CUPS no anunció colas configuradas.</p>
          )}
        </Accordion>
      </section>
      {value && (
        <section className="panel form-stack">
          <div className="section-heading">
            <h2>{value.id ? value.name : 'Nueva impresora'}</h2>
            <button className="secondary-button" onClick={() => setValue(null)}>
              Cerrar editor
            </button>
          </div>
          <Notice>
            {stale &&
              'La impresora cambió. Tus campos se conservan; revisa el destino actual antes de guardar.'}
          </Notice>
          {stale && current && (
            <button
              className="secondary-button"
              onClick={() => setValue({ ...value, revision: current.revision })}
            >
              Usar revisión vigente y conservar mis campos
            </button>
          )}
          <Form
            disabled={busy || stale}
            label="Guardar impresora"
            successMessage="Impresora guardada."
            onSave={async () =>
              setValue(
                await execute<Printer>(
                  value.id ? 'kitchen_save_printer' : 'kitchen_create_printer',
                  { id: value.id || undefined, body: { ...value } },
                ),
              )
            }
          >
            <TextField
              label="Nombre descriptivo de impresora"
              required
              value={value.name}
              onChange={(name) => setValue({ ...value, name })}
            />
            <Field label="Transporte de impresión">
              <Select<Printer['transport']>
                value={value.transport}
                options={[
                  { value: 'cups', label: 'Cola CUPS del servidor' },
                  { value: 'escpos_tcp', label: 'Impresora ESC/POS en red privada' },
                  { value: 'agent', label: 'Impresora de otra terminal · agente futuro' },
                ]}
                onChange={(transport) =>
                  setValue({
                    ...value,
                    transport,
                    destination: '',
                    cutter_option: '',
                    // La marca ESC/POS solo existe en CUPS: Go la rechaza en los demás transportes.
                    accepts_escpos: false,
                  })
                }
              />
            </Field>
            <TextField
              label={
                value.transport === 'cups'
                  ? 'Nombre de cola CUPS'
                  : value.transport === 'escpos_tcp'
                    ? 'IP privada de la impresora'
                    : 'Referencia del destino remoto'
              }
              required
              value={value.destination}
              onChange={(destination) => setValue({ ...value, destination })}
            />
            {value.transport === 'cups' && (
              <>
                <Check
                  label="Esta impresora entiende ESC/POS (formato y logotipo)"
                  value={value.accepts_escpos}
                  onChange={(accepts_escpos) => setValue({ ...value, accepts_escpos })}
                />
                <p className="muted text-sm">
                  Actívala solo si la impresora es térmica y entiende comandos ESC/POS. Así, los
                  documentos con negrita, tamaños o logotipo se le envían con formato cuando su cola
                  admite datos raw. Desactivada, o con una cola sin datos raw, la impresora recibe
                  solo el texto, sin formato ni logotipo, y el documento se imprime igual. Una
                  impresora de oficina o una que solo imprime con su controlador debe quedar
                  desactivada: con comandos ESC/POS imprimiría hojas en blanco o caracteres
                  extraños.
                </p>
              </>
            )}
            {value.transport === 'escpos_tcp' && (
              <>
                <IntegerField
                  label="Puerto ESC/POS"
                  min={9100}
                  max={9109}
                  value={value.port}
                  onChange={(port) => setValue({ ...value, port })}
                />
                <p className="muted text-sm">
                  El controlador utiliza CP850 para acentos españoles. Los caracteres fuera de esa
                  codificación se sustituyen por ?. Por red la impresora recibe comandos ESC/POS:
                  los documentos con negrita, tamaños o logotipo se imprimen con su formato.
                </p>
              </>
            )}
            {value.transport === 'agent' && (
              <Notice>
                Este destino requiere un agente que aún no existe. Sus trabajos permanecen en
                intervención; no se ejecutan comandos remotos.
              </Notice>
            )}
            <div className="form-grid">
              <Field label="Ancho de papel">
                <Select
                  value={String(value.paper_width)}
                  options={[
                    { value: '80', label: '80 mm' },
                    { value: '58', label: '58 mm' },
                  ]}
                  onChange={(width) =>
                    // Cada papel tiene su propia lista de columnas: al cambiarlo vuelven al
                    // predeterminado del papel nuevo (Go rechaza una combinación inválida).
                    setValue({ ...value, paper_width: Number(width), columns: null })
                  }
                />
              </Field>
              <IntegerField
                label="Copias por trabajo"
                min={1}
                max={3}
                value={value.copies}
                onChange={(copies) => setValue({ ...value, copies })}
              />
            </div>
            <PrinterColumns
              value={value}
              saved={current}
              canTest={!!value.id && can('kitchen.printers.manage')}
              busy={busy}
              execute={execute}
              onChange={(columns) => setValue({ ...value, columns })}
            />
            <Accordion
              title="Corte y respaldo"
              summary="Solo capacidades confirmadas del modelo y del controlador"
            >
              <Check
                label="El modelo y su controlador admiten corte"
                value={value.supports_cut}
                onChange={(supports_cut) =>
                  setValue({ ...value, supports_cut, cut: supports_cut && value.cut })
                }
              />
              <Check
                label="Solicitar corte por documento"
                value={value.cut}
                onChange={(cut) => setValue({ ...value, cut })}
              />
              {value.transport === 'cups' && value.cut && (
                <TextField
                  label="Opción de corte anunciada por CUPS"
                  value={value.cutter_option}
                  onChange={(cutter_option) => setValue({ ...value, cutter_option })}
                />
              )}
              <ReferenceField
                optional
                label="Impresora de respaldo explícita"
                value={value.fallback_id}
                options={(printers.data ?? []).filter((printer) => printer.id !== value.id)}
                onChange={(fallback_id) => setValue({ ...value, fallback_id: fallback_id || null })}
              />
              <p className="muted text-sm">
                El respaldo se usa tras tres errores confirmados sin envío. Un resultado incierto
                requiere decisión del operador.
              </p>
            </Accordion>
            <Check
              label="Impresora activa"
              value={value.active}
              onChange={(active) => setValue({ ...value, active })}
            />
          </Form>
          {value.id && can('kitchen.printers.test') && (
            <button
              className="secondary-button"
              disabled={busy}
              onClick={() =>
                void execute('kitchen_test_print', { body: { printer_id: value.id } })
                  .then(() =>
                    // «Enviada a la cola», nunca «impresa»: Go no confirma el papel.
                    notifySuccess(`Impresión de prueba enviada a la cola de ${value.name}.`),
                  )
                  .catch(() => undefined)
              }
            >
              Enviar una impresión de prueba a {value.name}
            </button>
          )}
        </section>
      )}
    </div>
  );
}

// Columnas de texto de la impresora (etapa 9C): lista cerrada por papel y, junto a la opción, la
// prueba de ancho con las columnas elegidas (aún sin guardar), para decidir tras verla en papel.
function PrinterColumns({
  value,
  saved,
  canTest,
  busy,
  execute,
  onChange,
}: {
  value: Printer;
  saved: Printer | undefined;
  canTest: boolean;
  busy: boolean;
  execute: ExecuteKitchen;
  onChange: (columns: number | null) => void;
}) {
  const choices = printerColumnChoices[value.paper_width] ?? printerColumnChoices[80]!;
  const [standard, wide] = choices;
  const selected = printerColumns(value);
  // La prueba usa el papel guardado: si se cambió en el formulario, primero hay que guardarlo.
  const paperPending = !!saved && saved.paper_width !== value.paper_width;
  async function printWidthTest() {
    const result = await execute<WidthTestResult>('kitchen_printer_width_test', {
      id: value.id,
      body: { columns: selected },
    });
    // «Enviada a la cola», nunca «impresa»: Go no confirma el papel.
    notifySuccess(
      `Prueba de ancho de ${result.columns} columnas enviada a la cola de ${value.name}.`,
    );
  }
  return (
    <div className="form-stack">
      {/* Un grupo de botones, no un campo: no va dentro de una etiqueta. */}
      <div className="field">
        <span>Columnas de texto</span>
        <SegmentedControl
          label="Columnas de texto"
          value={String(selected)}
          options={[
            { value: String(standard), label: `${standard} (predeterminado)` },
            { value: String(wide), label: `${wide} columnas` },
          ]}
          onChange={(choice) => onChange(Number(choice) === standard ? null : Number(choice))}
        />
      </div>
      <p className="muted text-sm">
        {standard} columnas es el formato de siempre en papel de {value.paper_width} mm. Usa {wide}{' '}
        solo si la prueba de ancho muestra completos el último número, la línea de guiones y el
        importe: muchas impresoras no imprimen todo el ancho del papel y cortarían el final del
        renglón. El cambio aplica a los documentos nuevos; las copias de documentos ya impresos
        conservan su ancho.
      </p>
      {canTest && (
        <div className="form-stack">
          <button
            type="button"
            className="secondary-button"
            disabled={busy || paperPending}
            onClick={() => void printWidthTest().catch(() => undefined)}
          >
            Imprimir prueba de ancho ({selected} columnas)
          </button>
          {paperPending && (
            <p className="muted text-sm">
              Guarda primero el nuevo ancho de papel para imprimir su prueba de ancho.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
