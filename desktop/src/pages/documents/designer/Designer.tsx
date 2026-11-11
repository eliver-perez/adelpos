import { useEffect, useRef, useState } from 'react';
import { useConfirmDialog } from '../../../components/ConfirmDialog';
import { IconLabel } from '../../../components/Icon';
import { useLeaveGuard } from '../../../hooks/useLeaveGuard';
import {
  documentPermissions,
  documentTemplate,
  templatesChangeEvent,
} from '../../../services/documents';
import type {
  CatalogKind,
  DocumentCatalog,
  PaperWidth,
  PrinterCompatibility,
  TemplateDetail,
  TemplatesChange,
  VersionDetail,
} from '../../../services/documents';
import { notifyError, notifyInfo, notifySuccess } from '../../../services/notify';
import { blockLabel } from './blockLabels';
import { BlockProperties, DocumentOptions } from './BlockProperties';
import { ConflictDialog } from './ConflictDialog';
import { ElementPalette } from './ElementPalette';
import type { PaletteItem } from './ElementPalette';
import { HistoryDialog } from './HistoryDialog';
import { PreviewPane } from './PreviewPane';
import type { PreviewMode } from './PreviewPane';
import { PrinterWarnings } from './PrinterSupport';
import { SampleDialog } from './SampleDialog';
import { SaveDialog } from './SaveDialog';
import { StructureList, dropIndexAt } from './StructureList';
import { restoreVersion, useDesignerSave } from './useDesignerSave';
import { useDesignerDraft } from './useDesignerDraft';
import { useOperations } from './useOperations';
import { usePreview } from './usePreview';

// Editor visual de una plantilla (etapa 9B, fase C), con tres áreas que aprovechan todo el
// monitor: elementos, vista previa y estructura con propiedades. En anchos menores las áreas pasan
// a pestañas sin perder el estado (todo vive aquí, no en cada área). Go es la autoridad: valida,
// renderiza la vista previa, guarda versiones inmutables y decide qué puede hacer cada persona.

type Area = 'palette' | 'preview' | 'structure';

interface PaletteDrag {
  item: PaletteItem;
  x: number;
  y: number;
  insertion: number | null;
}

export interface DesignerActions {
  activate: (template: TemplateDetail, unsavedDraft?: boolean) => Promise<void>;
  duplicate: (template: TemplateDetail) => void;
}

export function Designer({
  catalog,
  kind,
  template,
  permissions,
  timezone,
  printers,
  actions,
  onClose,
  onTemplateChanged,
}: {
  catalog: DocumentCatalog;
  kind: CatalogKind;
  template: TemplateDetail;
  permissions: string[];
  timezone: string;
  printers: PrinterCompatibility[] | null;
  actions: DesignerActions;
  onClose: () => void;
  onTemplateChanged: (template: TemplateDetail) => void;
}) {
  const can = (permission: string) => permissions.includes(permission);
  const { state, dispatch, dirty, selected } = useDesignerDraft(template);
  const base = state.base;
  const readOnly = base.system || !can(documentPermissions.manage);
  const run = useOperations();
  const { confirm, dialog } = useConfirmDialog();
  const [paperWidth, setPaperWidth] = useState<PaperWidth>(80);
  const [mode, setMode] = useState<PreviewMode>('sample');
  const [ticketID, setTicketID] = useState('');
  const [area, setArea] = useState<Area>('preview');
  const [saveOpen, setSaveOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [sampleOpen, setSampleOpen] = useState(false);
  const [external, setExternal] = useState<TemplateDetail | null>(null);
  const [drag, setDrag] = useState<PaletteDrag | null>(null);
  const [resolving, setResolving] = useState(false);
  const listRef = useRef<HTMLOListElement | null>(null);
  // Copia del arrastre en curso para leerla al soltar sin efectos dentro de setState.
  const dragRef = useRef<PaletteDrag | null>(null);
  dragRef.current = drag;

  const saver = useDesignerSave({
    base,
    draft: state.draft,
    run,
    dispatch,
    onSaved: (saved) => {
      setExternal(null);
      onTemplateChanged(saved);
    },
  });

  const source =
    mode === 'real' ? (ticketID ? { ticket_id: ticketID } : null) : { sample: true as const };
  const { preview, problem, loading } = usePreview(
    source ? { kind: kind.kind, layout: state.draft, paperWidth, source } : null,
  );

  // Una activación o un renombre confirmados desde la lista o el encabezado llegan como una lectura
  // nueva de la misma versión: solo cambia la base (revisión, nombre, activa), nunca el borrador.
  useEffect(() => {
    if (
      template.revision !== base.revision &&
      template.current_version.id === base.current_version.id
    ) {
      dispatch({ type: 'base', template });
    }
  }, [template, base.revision, base.current_version.id, dispatch]);

  useLeaveGuard(dirty, `Hay cambios sin guardar en la plantilla «${base.name}».`);

  // Aviso mientras se edita: otra persona guardó esta plantilla (evento documents.templates.changed).
  // Si no hay cambios locales se muestra la versión nueva; si los hay, el borrador se conserva y se
  // ofrecen las opciones del conflicto.
  useEffect(() => {
    async function changed(event: Event) {
      const detail = (event as CustomEvent<TemplatesChange>).detail;
      if (!detail?.templates.includes(base.id) || saver.saving.current) {
        return;
      }
      try {
        const latest = await documentTemplate(base.id);
        if (latest.revision === base.revision) {
          return;
        }
        if (latest.current_version.id === base.current_version.id) {
          // Solo cambió el nombre o la activación: el contenido es el mismo.
          dispatch({ type: 'base', template: latest });
          return;
        }
        if (!dirty) {
          dispatch({ type: 'reset', template: latest });
          notifyInfo(
            `Otra persona guardó la versión ${latest.current_version.number} de «${latest.name}»: se muestra la versión nueva.`,
          );
          return;
        }
        setExternal(latest);
      } catch {
        // La consulta se repite con el siguiente evento; el guardado vuelve a comprobar la revisión.
      }
    }
    window.addEventListener(templatesChangeEvent, changed);
    return () => window.removeEventListener(templatesChangeEvent, changed);
  }, [base.id, base.revision, base.current_version.id, dirty, dispatch, saver.saving]);

  // Arrastre desde el panel de elementos: el editor sigue el puntero y, al soltar sobre la
  // estructura, inserta el bloque en esa posición.
  const dragging = drag !== null;
  useEffect(() => {
    if (!dragging) {
      return;
    }
    function positionOver(x: number, y: number) {
      const list = listRef.current;
      if (!list) {
        return null;
      }
      const box = list.getBoundingClientRect();
      const inside = x >= box.left && x <= box.right && y >= box.top - 24 && y <= box.bottom + 24;
      return inside ? dropIndexAt(list, y) : null;
    }
    function move(event: PointerEvent) {
      const insertion = positionOver(event.clientX, event.clientY);
      setDrag((current) =>
        current ? { ...current, x: event.clientX, y: event.clientY, insertion } : current,
      );
    }
    function up(event: PointerEvent) {
      const insertion = positionOver(event.clientX, event.clientY);
      const current = dragRef.current;
      setDrag(null);
      if (current && insertion !== null) {
        dispatch({
          type: 'add',
          definition: current.item.definition,
          kind,
          shortcut: current.item.shortcut,
          index: insertion,
        });
      }
    }
    function cancel() {
      setDrag(null);
    }
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', cancel);
    return () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', cancel);
    };
  }, [dragging, dispatch, kind]);

  function blockName(blockID: string) {
    const block = state.draft.blocks.find((item) => item.id === blockID);
    return block ? blockLabel(kind, block) : 'el bloque';
  }

  function select(blockID: string) {
    if (!state.draft.blocks.some((block) => block.id === blockID)) {
      return;
    }
    dispatch({ type: 'select', id: blockID });
    setArea('structure');
  }

  function add(item: PaletteItem) {
    dispatch({ type: 'add', definition: item.definition, kind, shortcut: item.shortcut });
    setArea('structure');
  }

  async function leave() {
    if (dirty) {
      const confirmed = await confirm({
        title: 'Salir sin guardar',
        message: (
          <p>
            Tienes cambios sin guardar en «{base.name}». Si sales ahora, se descartan; las versiones
            guardadas no cambian.
          </p>
        ),
        confirmLabel: 'Salir sin guardar',
        cancelLabel: 'Seguir editando',
        tone: 'danger',
      });
      if (!confirmed) {
        return;
      }
    }
    onClose();
  }

  async function duplicate() {
    if (dirty) {
      const confirmed = await confirm({
        title: 'Duplicar la versión guardada',
        message: (
          <p>
            La copia parte de la versión {base.current_version.number} guardada. Tus cambios sin
            guardar no se copian y se descartan al abrir la copia.
          </p>
        ),
        confirmLabel: 'Duplicar sin mis cambios',
        cancelLabel: 'Seguir editando',
        tone: 'danger',
      });
      if (!confirmed) {
        return;
      }
    }
    actions.duplicate(base);
  }

  async function saveWithNote(note: string) {
    // Con éxito o con rechazo el diálogo se cierra: el resultado lo explica el aviso, el bloque
    // señalado o el diálogo del conflicto.
    await saver.save(note);
    setSaveOpen(false);
  }

  async function restore(version: VersionDetail) {
    try {
      const saved = await restoreVersion(run, base, version);
      dispatch({ type: 'saved', template: saved });
      setHistoryOpen(false);
      notifySuccess(
        `Versión ${version.number} restaurada como versión ${saved.current_version.number}.`,
      );
      onTemplateChanged(saved);
    } catch (error) {
      notifyError(error, 'No se pudo restaurar la versión.');
    }
  }

  async function resolveSaveMine() {
    setResolving(true);
    try {
      await saver.saveOverLatest();
    } finally {
      setResolving(false);
    }
  }

  const invalidID = problem?.blockID || saver.problem?.blockID || '';
  const invalidMessage =
    problem?.blockID === selected?.id
      ? (problem?.message ?? '')
      : saver.problem?.blockID === selected?.id
        ? (saver.problem?.message ?? '')
        : '';
  const marks = {
    empty: new Map((preview?.empty_blocks ?? []).map((item) => [item.block_id, item.reason])),
    invalidID,
  };
  const formatted = !!preview?.lines.some((line) => line.bold || line.size !== 'normal');
  const withLogo = !!preview?.lines.some((line) => line.image);
  const canSample = can(documentPermissions.samples);
  const conflict = saver.conflict;

  return (
    <section className="designer" aria-label={`Editor de ${base.name}`}>
      <header className="designer-header">
        <button type="button" className="secondary-button" onClick={() => void leave()}>
          <IconLabel icon="back">Volver a plantillas</IconLabel>
        </button>
        <div className="designer-heading">
          <h1>{base.name}</h1>
          <p className="muted text-xs">
            {kind.name} · versión {base.current_version.number} guardada
            {base.active ? ' · activa' : ' · inactiva'}
            {base.system ? ' · predeterminada del sistema, solo lectura' : ''}
          </p>
        </div>
        <span className={`designer-dirty ${dirty ? 'designer-dirty-on' : ''}`} role="status">
          {readOnly ? 'Solo lectura' : dirty ? 'Cambios sin guardar' : 'Sin cambios pendientes'}
        </span>
        <div className="designer-header-actions">
          <button type="button" className="secondary-button" onClick={() => setHistoryOpen(true)}>
            <IconLabel icon="clock">Historial</IconLabel>
          </button>
          {canSample && (
            <button type="button" className="secondary-button" onClick={() => setSampleOpen(true)}>
              <IconLabel icon="printer">Imprimir muestra</IconLabel>
            </button>
          )}
          {can(documentPermissions.manage) && (
            <button type="button" className="secondary-button" onClick={() => void duplicate()}>
              Duplicar
            </button>
          )}
          {can(documentPermissions.manage) && !base.active && (
            <button
              type="button"
              className="secondary-button"
              onClick={() => void actions.activate(base, dirty)}
            >
              <IconLabel icon="check">Activar</IconLabel>
            </button>
          )}
          {!readOnly && (
            <button
              type="button"
              className="primary-button"
              disabled={!dirty}
              onClick={() => setSaveOpen(true)}
            >
              Guardar versión
            </button>
          )}
        </div>
      </header>
      {base.system && (
        <p className="designer-banner">
          La plantilla predeterminada del sistema reproduce el formato original y no se edita.
          {can(documentPermissions.manage)
            ? ' Duplícala para crear tu propia versión.'
            : ' Puedes consultarla y ver su vista previa.'}
        </p>
      )}
      {!base.system && readOnly && (
        <p className="designer-banner">
          Solo lectura: editar plantillas requiere el permiso «Administrar plantillas de
          documentos».
        </p>
      )}
      {external && (
        <div className="designer-banner designer-banner-warning" role="status">
          <span>
            Otra persona guardó la versión {external.current_version.number} mientras editabas. Tu
            borrador se conserva.
          </span>
          <button
            type="button"
            className="secondary-button"
            onClick={() => saver.setConflict(external)}
          >
            Ver opciones
          </button>
        </div>
      )}
      <div className="catalog-tabs designer-tabs" role="tablist" aria-label="Áreas del editor">
        {(
          [
            ['palette', 'Elementos'],
            ['preview', 'Vista previa'],
            ['structure', 'Estructura y propiedades'],
          ] as [Area, string][]
        )
          // En solo lectura no hay elementos que agregar: el área no se muestra.
          .filter(([id]) => !readOnly || id !== 'palette')
          .map(([id, label]) => (
            <button
              type="button"
              role="tab"
              key={id}
              aria-selected={area === id}
              className={area === id ? 'tab-active' : ''}
              onClick={() => setArea(id)}
            >
              {label}
            </button>
          ))}
      </div>
      <div className="designer-grid" data-readonly={readOnly}>
        <div className="designer-area designer-area-palette" data-active={area === 'palette'}>
          <h2 className="designer-area-title">Elementos</h2>
          {readOnly ? (
            <p className="muted text-sm">
              {base.system && can(documentPermissions.manage)
                ? 'Duplica la plantilla para agregar elementos.'
                : 'Los elementos se agregan al editar una plantilla del restaurante.'}
            </p>
          ) : (
            <ElementPalette
              kind={kind}
              blocks={state.draft.blocks}
              limit={catalog.limits.blocks}
              onAdd={add}
              onDragStart={(item, x, y) => setDrag({ item, x, y, insertion: null })}
            />
          )}
        </div>
        <div className="designer-area designer-area-preview" data-active={area === 'preview'}>
          <PreviewPane
            kind={kind.kind}
            preview={preview}
            problem={problem}
            loading={loading}
            paperWidth={paperWidth}
            mode={mode}
            ticketID={ticketID}
            canPreviewReal={can(documentPermissions.previewReal)}
            selectedID={state.selectedID}
            timezone={timezone}
            blockName={blockName}
            onPaperWidth={setPaperWidth}
            onMode={(next) => {
              setMode(next);
              setTicketID('');
            }}
            onTicket={setTicketID}
            onSelect={select}
          />
          <PrinterWarnings printers={printers} formatted={formatted} withLogo={withLogo} />
        </div>
        <div className="designer-area designer-area-structure" data-active={area === 'structure'}>
          <div className="designer-side-section">
            <h2 className="designer-area-title">Estructura</h2>
            <DocumentOptions
              kind={kind}
              layout={state.draft}
              readOnly={readOnly}
              dispatch={dispatch}
            />
            <StructureList
              kind={kind}
              blocks={state.draft.blocks}
              selectedID={state.selectedID}
              readOnly={readOnly}
              marks={marks}
              removed={state.removed}
              externalInsertion={drag?.insertion ?? null}
              listRef={listRef}
              onSelect={(id) => dispatch({ type: 'select', id })}
              onMove={(from, to) => dispatch({ type: 'move', from, to })}
              onRemove={(id) => dispatch({ type: 'remove', id })}
              onUndoRemove={() => dispatch({ type: 'undo-remove' })}
            />
          </div>
          <div className="designer-side-section">
            <h2 className="designer-area-title">Propiedades</h2>
            <BlockProperties
              catalog={catalog}
              kind={kind}
              block={selected}
              layout={state.draft}
              readOnly={readOnly}
              problem={invalidMessage}
              dispatch={dispatch}
            />
          </div>
        </div>
      </div>
      {drag && (
        <div
          className="designer-drag-ghost"
          style={{ left: drag.x + 12, top: drag.y + 12 }}
          aria-hidden="true"
        >
          {drag.item.name}
        </div>
      )}
      {saveOpen && (
        <SaveDialog
          nextVersion={base.current_version.number + 1}
          active={base.active}
          onSave={saveWithNote}
          onClose={() => setSaveOpen(false)}
        />
      )}
      {historyOpen && (
        <HistoryDialog
          template={base}
          timezone={timezone}
          canRestore={!readOnly}
          restoreBlocked={dirty ? 'Guarda o descarta tu borrador antes de restaurar.' : ''}
          onRestore={restore}
          onClose={() => setHistoryOpen(false)}
        />
      )}
      {sampleOpen && (
        <SampleDialog
          kind={kind.kind}
          layout={state.draft}
          printers={printers}
          canWatchQueue={can('kitchen.queue.read') && can('kitchen.stations.all')}
          run={run}
          onClose={() => setSampleOpen(false)}
        />
      )}
      {conflict && (
        <ConflictDialog
          latest={conflict}
          timezone={timezone}
          busy={resolving}
          onSaveMine={() => void resolveSaveMine()}
          onDiscardMine={() => {
            saver.discardDraft();
            setExternal(null);
            notifyInfo('Se descartó tu borrador: se muestra la versión más reciente.');
          }}
          onKeepEditing={() => {
            setExternal(conflict);
            saver.setConflict(null);
          }}
        />
      )}
      {dialog}
    </section>
  );
}
