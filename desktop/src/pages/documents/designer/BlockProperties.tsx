import { SegmentedControl } from '../../../components/SegmentedControl';
import { ToggleButton } from '../../../components/ToggleButton';
import type {
  BlockAlign,
  BlockSize,
  CatalogKind,
  DocumentCatalog,
  TemplateBlock,
  TemplateLayout,
} from '../../../services/documents';
import { notifyInfo } from '../../../services/notify';
import { blockLabel, catalogBlock } from './blockLabels';
import { fieldItems } from './draft';
import { OptionControl } from './OptionControl';
import { Stepper, linesUnit } from './Stepper';
import type { DesignerDispatch } from './useDesignerDraft';

// Propiedades del bloque elegido: solo las que el catálogo de Go declara para su tipo (el
// logotipo no tiene alineación ni tamaño propios; el espacio solo se oculta o se muestra) y sus
// opciones con controles táctiles. Los tamaños, la negrita y el logotipo dependen de la impresora:
// se explica junto a los controles.

const textSizeLabels: Record<string, string> = {
  inherit: 'Según Cocina',
  normal: 'Normal',
  large: 'Grande',
};

export function DocumentOptions({
  kind,
  layout,
  readOnly,
  dispatch,
}: {
  kind: CatalogKind;
  layout: TemplateLayout;
  readOnly: boolean;
  dispatch: DesignerDispatch;
}) {
  if (kind.text_sizes.length <= 1) {
    return null;
  }
  return (
    <div className="designer-control">
      <span className="designer-control-label">Tamaño de texto de todo el documento</span>
      <SegmentedControl
        label="Tamaño de texto de todo el documento"
        value={layout.options.text_size}
        options={kind.text_sizes.map((size) => ({
          value: size,
          label: textSizeLabels[size] ?? size,
        }))}
        disabled={readOnly}
        onChange={(value) => dispatch({ type: 'text-size', value })}
      />
      <p className="muted text-xs">
        «Según Cocina» usa la opción «Tamaño de texto» de Cocina. Con texto grande, las columnas se
        reducen a la mitad y se ignoran los tamaños de cada bloque (la negrita se conserva).
      </p>
    </div>
  );
}

export function BlockProperties({
  catalog,
  kind,
  block,
  layout,
  readOnly,
  problem,
  dispatch,
}: {
  catalog: DocumentCatalog;
  kind: CatalogKind;
  block: TemplateBlock | null;
  layout: TemplateLayout;
  readOnly: boolean;
  // Mensaje de Go si este bloque no pasó la validación.
  problem: string;
  dispatch: DesignerDispatch;
}) {
  if (!block) {
    return (
      <section className="designer-properties" aria-label="Propiedades del bloque">
        <p className="muted">Elige un bloque en la estructura o en la vista previa.</p>
      </section>
    );
  }
  const definition = catalogBlock(kind, block.type);
  const properties = new Set(definition?.properties ?? []);
  const required = !!definition?.required;
  const largeText = layout.options.text_size === 'large';
  const id = block.id;

  function change(update: Partial<Omit<TemplateBlock, 'id' | 'type'>>) {
    dispatch({ type: 'update', id, change: update });
  }

  function changeOption(key: string, value: unknown) {
    // «Etiqueta y valor» admite un solo dato: se conserva el primero y se avisa.
    if (block && key === 'layout' && value === 'pair' && fieldItems(block).length > 1) {
      dispatch({ type: 'option', id, key: 'items', value: fieldItems(block).slice(0, 1) });
      notifyInfo('La disposición «etiqueta y valor» admite un solo dato: se conservó el primero.');
    }
    dispatch({ type: 'option', id, key, value });
  }

  return (
    <section className="designer-properties" aria-label="Propiedades del bloque">
      <div className="designer-properties-heading">
        <h3>{blockLabel(kind, block)}</h3>
        {definition?.description && <p className="muted text-xs">{definition.description}</p>}
      </div>
      {problem && (
        <p className="checkout-inline-note" role="alert">
          {problem}
        </p>
      )}
      {required && (
        <p className="muted text-xs">
          Obligatorio: se puede mover y cambiar su alineación, tamaño y énfasis, pero no quitar ni
          ocultar.
        </p>
      )}
      {properties.has('visible') && (
        <ToggleButton
          label="Visible"
          pressed={block.visible}
          disabled={readOnly || required}
          onChange={(visible) => change({ visible })}
        />
      )}
      {properties.has('align') && (
        <div className="designer-control">
          <span className="designer-control-label">Alineación</span>
          <SegmentedControl
            label="Alineación"
            value={block.align}
            options={catalog.properties.align.map((item) => ({
              value: item.value as BlockAlign,
              label: item.label,
            }))}
            disabled={readOnly}
            onChange={(align) => change({ align })}
          />
        </div>
      )}
      {properties.has('size') && (
        <div className="designer-control">
          <span className="designer-control-label">Tamaño</span>
          <SegmentedControl
            label="Tamaño"
            value={block.size}
            options={catalog.properties.size.map((item) => ({
              value: item.value as BlockSize,
              label: item.label,
            }))}
            disabled={readOnly}
            onChange={(size) => change({ size })}
          />
          {largeText && (
            <p className="muted text-xs">
              Con el texto grande del documento este tamaño no se aplica.
            </p>
          )}
        </div>
      )}
      {(properties.has('bold') || properties.has('uppercase')) && (
        <div className="button-row">
          {properties.has('bold') && (
            <ToggleButton
              label="Negrita"
              pressed={block.bold}
              disabled={readOnly}
              onChange={(bold) => change({ bold })}
            />
          )}
          {properties.has('uppercase') && (
            <ToggleButton
              label="Mayúsculas"
              pressed={block.uppercase}
              disabled={readOnly}
              onChange={(uppercase) => change({ uppercase })}
            />
          )}
        </div>
      )}
      {(properties.has('size') || properties.has('bold') || block.type === 'logo') && (
        <p className="designer-printer-note text-xs">
          Tamaños, negrita y logotipo requieren una impresora ESC/POS por red o una cola CUPS que
          admita datos raw; en otra cola se imprime el mismo texto sin ese formato.
        </p>
      )}
      {(properties.has('spacing_before') || properties.has('spacing_after')) && (
        <div className="designer-steppers">
          {properties.has('spacing_before') && (
            <Stepper
              label="Espacio antes"
              value={block.spacing_before}
              min={catalog.properties.spacing.min}
              max={catalog.properties.spacing.max}
              unit={linesUnit}
              disabled={readOnly}
              onChange={(spacing_before) => change({ spacing_before })}
            />
          )}
          {properties.has('spacing_after') && (
            <Stepper
              label="Espacio después"
              value={block.spacing_after}
              min={catalog.properties.spacing.min}
              max={catalog.properties.spacing.max}
              unit={linesUnit}
              disabled={readOnly}
              onChange={(spacing_after) => change({ spacing_after })}
            />
          )}
        </div>
      )}
      {(definition?.options ?? []).map((spec) => (
        <OptionControl
          key={spec.key}
          spec={spec}
          block={block}
          kind={kind}
          catalog={catalog}
          disabled={readOnly}
          onChange={changeOption}
        />
      ))}
    </section>
  );
}
