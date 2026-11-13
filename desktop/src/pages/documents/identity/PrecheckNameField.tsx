import { Field } from '../../../components/Forms';
import { Select } from '../../../components/Select';
import { precheckNameOptions, precheckNames } from '../../../services/documentNames';
import type { PrecheckNameKey } from '../../../services/documentNames';

// Nombre visible del documento previo al cobro (etapa 9F, §4.5). Desde la revisión 9F (M3) se elige aquí,
// en Administración › Documentos, como pide la especificación («en la configuración de documentos»), y se
// guarda con el resto de la identidad (permiso «Modificar la identidad documental y el logotipo del
// establecimiento»). Es el mismo documento con las mismas reglas: cambian los botones, los avisos y el
// encabezado impreso; la leyenda de que no es un comprobante fiscal se conserva.
export function PrecheckNameField({
  value,
  readOnly,
  onChange,
}: {
  value: PrecheckNameKey;
  readOnly: boolean;
  onChange: (value: PrecheckNameKey) => void;
}) {
  const current = precheckNames[value] ?? precheckNames.precheck;
  return (
    <section className="panel form-stack" aria-labelledby="identity-precheck-title">
      <div>
        <h2 id="identity-precheck-title">Documento previo al cobro</h2>
        <p className="muted text-sm">
          Así se llama el documento que se entrega antes de cobrar en los botones, en los avisos y
          en su encabezado impreso ({current.printed}). Es el mismo documento con las mismas reglas
          y no es un comprobante fiscal. El ticket que se entrega después de cobrar se llama «Ticket
          de pago».
        </p>
      </div>
      <Field label="Nombre de la precuenta">
        <Select<PrecheckNameKey>
          value={value}
          options={precheckNameOptions}
          disabled={readOnly}
          onChange={onChange}
        />
      </Field>
    </section>
  );
}
