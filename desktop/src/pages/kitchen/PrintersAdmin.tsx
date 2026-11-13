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
  cutFeedLineChoices,
  cutFeedSummary,
  defaultCutFeedLines,
  defaultCutFor,
  previousCutFeedChoice,
  printerColumnChoices,
  printerColumns,
  type ExecuteKitchen,
  type Printer,
  type WidthTestResult,
} from '../../services/kitchen';
import { usePrecheckName } from '../../services/documentNames';
import { notifySuccess } from '../../services/notify';
import { Select } from '../../components/Select';
import { isWindows } from '../../commands/platform';

// Impresora nueva (etapa 9F, decisión D5): en 80 mm corta el papel al terminar, corte total con 4
// renglones de avance; al elegir 58 mm el corte se desactiva hasta que el propietario lo active.
const emptyPrinter = (): Printer => ({
  id: '',
  name: '',
  transport: 'cups',
  destination: '',
  port: 9100,
  paper_width: 80,
  copies: 1,
  supports_cut: defaultCutFor(80),
  cut: defaultCutFor(80),
  cut_mode: 'full',
  cut_feed_lines: defaultCutFeedLines,
  cutter_option: '',
  accepts_escpos: false,
  columns: null,
  active: true,
  fallback_id: null,
  revision: 0,
});

// Opciones de los renglones de avance: la lista cerrada de 0 a 8 y, solo para una impresora que ya
// cortaba antes de la 9F y conserva su final, «Como hasta ahora».
function cutFeedOptions(previousEnding: boolean) {
  const options = cutFeedLineChoices.map((lines) => ({
    value: String(lines),
    label: lines === defaultCutFeedLines ? `${lines} (predeterminado)` : String(lines),
  }));
  if (previousEnding) {
    options.unshift({ value: previousCutFeedChoice, label: 'Como hasta ahora' });
  }
  return options;
}

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
  const [value, setValueState] = useState<Printer | null>(null);
  // Nombre visible de la precuenta (etapa 9F, §4.5) en la explicación del corte.
  const precheck = usePrecheckName();
  // El corte de una impresora nueva sigue a su papel hasta que la persona lo elige.
  const [cutChosen, setCutChosen] = useState(false);
  const setValue = (next: Printer | null) => {
    if (!next || next.id !== value?.id || (!next.id && !value)) {
      setCutChosen(false);
    }
    setValueState(next);
  };
  const current = printers.data?.find((entry) => entry.id === value?.id);
  const stale = !!value?.id && current?.revision !== value.revision;
  // Etapa 9E: en Windows la cola de impresión del equipo es la de las impresoras instaladas en
  // Windows (siempre ESC/POS sin convertir), no CUPS; en macOS los textos no cambian.
  const windows = isWindows();
  return (
    <div className="kitchen-editor-layout">
      <section className="panel form-stack">
        <div className="section-heading">
          <h2>Impresoras</h2>
          <button className="primary-button" onClick={() => setValue(emptyPrinter())}>
            Registrar impresora
          </button>
        </div>
        <Notice>{printers.error}</Notice>
        {windows ? (
          <p className="muted">
            Se usan las impresoras instaladas en Windows en el equipo servidor y las impresoras de
            tickets conectadas a la red. Una impresora conectada a otra terminal todavía no se puede
            usar: conéctala a la red o al equipo servidor.
          </p>
        ) : (
          <p className="muted">
            Se usan las impresoras instaladas en el equipo servidor (colas CUPS, el sistema de
            impresión de macOS) y las impresoras de tickets conectadas a la red. Una impresora
            conectada a otra terminal todavía no se puede usar: conéctala a la red o al equipo
            servidor.
          </p>
        )}
        {printers.data?.map((printer) => (
          <button
            className="kitchen-list-button"
            key={printer.id}
            onClick={() => setValue(printer)}
          >
            <strong>{printer.name}</strong>
            <span>
              {printer.transport === 'cups'
                ? windows
                  ? 'Impresora instalada en Windows'
                  : 'CUPS del equipo servidor'
                : printer.transport === 'escpos_tcp'
                  ? 'ESC/POS por red'
                  : 'Otra terminal (aún no disponible)'}{' '}
              · {printer.paper_width} mm · {printerColumns(printer)} columnas ·{' '}
              {printer.active ? 'Activa' : 'Inactiva'}
            </span>
          </button>
        ))}
        <Accordion
          title={windows ? 'Impresoras instaladas en Windows' : 'Colas anunciadas por CUPS'}
          summary={
            windows
              ? 'Que una impresora aparezca no confirma que tenga papel ni esté conectada'
              : 'Que una cola aparezca no confirma que la impresora tenga papel ni esté conectada'
          }
        >
          <button className="secondary-button" onClick={() => void discovery.reload()}>
            Consultar impresoras del equipo servidor
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
                {windows ? 'Usar esta impresora' : 'Usar esta cola'}
              </button>
              {/* Windows no anuncia opciones de controlador: el resumen ya dice puerto y estado. */}
              {!windows && (
                <Accordion title={`Opciones del controlador ${printer.queue}`}>
                  <pre className="kitchen-document">
                    {printer.options || 'El controlador no anunció opciones adicionales.'}
                  </pre>
                </Accordion>
              )}
            </div>
          ))}
          {discovery.data?.length === 0 && (
            <p className="muted">
              {windows
                ? 'Windows no tiene impresoras instaladas en el equipo servidor.'
                : 'CUPS no anunció colas configuradas.'}
            </p>
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
              Usar la versión vigente y conservar mis campos
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
                  {
                    value: 'cups',
                    label: windows
                      ? 'Impresora instalada en Windows en el equipo servidor'
                      : 'Cola CUPS del equipo servidor',
                  },
                  { value: 'escpos_tcp', label: 'Impresora ESC/POS en red privada' },
                  { value: 'agent', label: 'Impresora de otra terminal (aún no disponible)' },
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
                  ? windows
                    ? 'Nombre de la impresora en Windows'
                    : 'Nombre de cola CUPS'
                  : value.transport === 'escpos_tcp'
                    ? 'IP privada de la impresora'
                    : 'Referencia del destino remoto'
              }
              required
              value={value.destination}
              onChange={(destination) => setValue({ ...value, destination })}
            />
            {/* En Windows la impresora recibe siempre ESC/POS sin convertir: la marca no aplica. */}
            {value.transport === 'cups' && windows && (
              <p className="muted text-sm">
                En Windows la impresora recibe los comandos ESC/POS (el lenguaje de las impresoras
                de tickets) sin convertir: los documentos con negrita, tamaños o logotipo se
                imprimen con su formato y los acentos y la ñ con la tabla de caracteres CP850 de la
                impresora. Usa solo impresoras térmicas de tickets instaladas con un puerto USB,
                serie, paralelo o TCP/IP estándar: una impresora de oficina imprimiría hojas en
                blanco o caracteres extraños. AD-EL POS revisa la impresora antes de cada envío y no
                envía nada a una impresora virtual (PDF, XPS), a un archivo ni a una impresora de
                otro equipo.
              </p>
            )}
            {value.transport === 'cups' && !windows && (
              <>
                <Check
                  label="Esta impresora entiende ESC/POS (formato y logotipo)"
                  value={value.accepts_escpos}
                  onChange={(accepts_escpos) => setValue({ ...value, accepts_escpos })}
                />
                <p className="muted text-sm">
                  Actívala solo si la impresora es térmica y entiende comandos ESC/POS. Así, los
                  documentos con negrita, tamaños o logotipo se le envían con formato cuando su cola
                  acepta datos sin convertir («raw»). Desactivada, o con una cola que los convierte,
                  la impresora recibe solo el texto, sin formato ni logotipo, y el documento se
                  imprime igual. Una impresora de oficina o una que solo imprime con su controlador
                  debe quedar desactivada: con comandos ESC/POS imprimiría hojas en blanco o
                  caracteres extraños.
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
                  Los acentos y la ñ se imprimen con la tabla de caracteres CP850 de la impresora;
                  los caracteres que no existen en ella salen como «?». Por red la impresora recibe
                  comandos ESC/POS: los documentos con negrita, tamaños o logotipo se imprimen con
                  su formato.
                </p>
              </>
            )}
            {value.transport === 'agent' && (
              <Notice>
                Imprimir en una impresora de otra terminal aún no está disponible. Sus trabajos
                quedan en espera de revisión y no se envía nada a esa terminal.
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
                  onChange={(width) => {
                    // Cada papel tiene su propia lista de columnas: al cambiarlo vuelven al
                    // predeterminado del papel nuevo (Go rechaza una combinación inválida). En una
                    // impresora nueva, el corte sigue al papel mientras no se haya elegido.
                    const paper = Number(width);
                    const cut = !value.id && !cutChosen ? defaultCutFor(paper) : value.cut;
                    setValueState({
                      ...value,
                      paper_width: paper,
                      columns: null,
                      cut,
                      supports_cut: cut,
                    });
                  }}
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
              title="Corte de papel y respaldo"
              summary={
                value.cut
                  ? `Corta al terminar · ${value.cut_mode === 'partial' ? 'parcial' : 'total'} · ${cutFeedSummary(value.cut_feed_lines)}`
                  : 'Sin corte al terminar'
              }
              defaultOpen={!value.id}
            >
              <Check
                label="Cortar el papel al terminar"
                value={value.cut}
                onChange={(cut) => {
                  setCutChosen(true);
                  // Una sola casilla (etapa 9F): Go guarda «el modelo admite corte» igual a ésta. Una
                  // impresora que no cortaba y empieza a cortar recibe los renglones predeterminados.
                  const startsCutting = cut && value.cut_feed_lines === null && !current?.cut;
                  setValueState({
                    ...value,
                    cut,
                    supports_cut: cut,
                    cut_feed_lines: startsCutting ? defaultCutFeedLines : value.cut_feed_lines,
                  });
                }}
              />
              {value.cut && (
                <>
                  <div className="field">
                    <span>Tipo de corte</span>
                    <SegmentedControl
                      label="Tipo de corte"
                      value={value.cut_mode}
                      options={[
                        { value: 'full', label: 'Total' },
                        { value: 'partial', label: 'Parcial' },
                      ]}
                      onChange={(cut_mode) =>
                        setValueState({ ...value, cut_mode: cut_mode as Printer['cut_mode'] })
                      }
                    />
                  </div>
                  <Field label="Renglones de avance antes del corte">
                    <Select
                      value={
                        value.cut_feed_lines === null
                          ? previousCutFeedChoice
                          : String(value.cut_feed_lines)
                      }
                      options={cutFeedOptions(!!current?.cut && current.cut_feed_lines === null)}
                      onChange={(lines) =>
                        setValueState({
                          ...value,
                          cut_feed_lines: lines === previousCutFeedChoice ? null : Number(lines),
                        })
                      }
                    />
                  </Field>
                </>
              )}
              <p className="muted text-sm">
                Al final de cada documento (tickets, {precheck.plural}, comandas, avisos y cortes de
                caja) la impresora deja en blanco los renglones elegidos después del último renglón,
                para que el corte no caiga sobre el texto, y corta: total separa el papel; parcial
                deja una pequeña unión. «Como hasta ahora» conserva el final de las versiones
                anteriores en una impresora que ya cortaba. La mayoría de las impresoras sin
                cortador ignoran la orden de corte; pruébalo con «Imprimir prueba de ancho», que
                también corta. Las de 58 mm muchas veces no tienen cortador: por eso el corte
                empieza desactivado en ese papel.
              </p>
              {/* En Windows el corte es el comando ESC/POS de la impresora, no una opción de CUPS. */}
              {value.transport === 'cups' && value.cut && !windows && (
                <>
                  <TextField
                    label="Opción de corte anunciada por CUPS"
                    value={value.cutter_option}
                    onChange={(cutter_option) => setValueState({ ...value, cutter_option })}
                  />
                  <p className="muted text-sm">
                    En una cola de macOS que no recibe los comandos de la impresora («Esta impresora
                    entiende ESC/POS» desactivada), el corte lo hace su controlador con esta opción.
                    Sin ella, el documento se imprime sin corte.
                  </p>
                </>
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
