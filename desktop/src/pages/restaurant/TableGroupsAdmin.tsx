import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { TextField } from '../../components/catalog/EditorFields';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Form } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { MultiSelect } from '../../components/Select';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import type { Overview, TableGroup } from '../../services/restaurant';
import { uncertainError } from './capture/draftSync';
import type { ExecuteMutation } from './OperationsAdmin';

// Mesas divisibles en Áreas y distribución (etapa M1, §3; M1-D4): un grupo predefinido («Mesa 3») formado por varias
// mesas normales del área (sus secciones, 3A, 3B y 3C), que se dibujan juntas en el mapa. Cada sección sigue siendo
// una mesa normal y nunca se renombra aquí; el atajo «Unir Mesa 3» las une de un toque. Go valida todo (mismo área,
// nombre libre, una mesa en un solo grupo, grupo sin mesas unidas) y sus rechazos se muestran tal cual.

interface GroupDraft {
  id: string;
  name: string;
  table_ids: string[];
  revision: number;
}

const newGroup = (): GroupDraft => ({ id: '', name: '', table_ids: [], revision: 0 });

export function TableGroupsAdmin({
  overview,
  areaID,
  execute,
  busy,
}: {
  overview: Overview;
  areaID: string;
  execute: ExecuteMutation;
  busy: boolean;
}) {
  const [draft, setDraft] = useState<GroupDraft | null>(null);
  const { confirm, dialog } = useConfirmDialog();
  const area = overview.areas.find((entry) => entry.id === areaID);
  const groups = (overview.groups ?? []).filter((group) => group.area_id === areaID);
  const tables = overview.tables.filter((table) => table.area_id === areaID && table.active);
  const tableName = (id: string) =>
    overview.tables.find((table) => table.id === id)?.name ?? 'Mesa desactivada';

  async function save(current: GroupDraft) {
    const body = { name: current.name, table_ids: current.table_ids };
    const saved = current.id
      ? await execute<{ group: TableGroup }>('restaurant_save_table_group', {
          id: current.id,
          body: { ...body, revision: current.revision },
        })
      : await execute<{ group: TableGroup }>('restaurant_create_table_group', {
          body: { ...body, area_id: areaID },
        });
    setDraft({
      id: saved.group.id,
      name: saved.group.name,
      table_ids: saved.group.table_ids,
      revision: saved.group.revision,
    });
  }

  // Quitar un grupo (PATCH active: false, M1-D4): sus mesas siguen como mesas normales y el nombre queda libre. Un
  // grupo quitado no se vuelve a activar: se crea otro.
  async function remove(group: TableGroup) {
    const confirmed = await confirm({
      title: `¿Quitar ${group.name}?`,
      message:
        'Sus mesas siguen como mesas normales, con sus nombres y sus cuentas. Para volver a tenerla, crea el grupo otra vez.',
      confirmLabel: 'Quitar grupo',
      cancelLabel: 'Conservarlo',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    try {
      await execute('restaurant_save_table_group', {
        id: group.id,
        body: { revision: group.revision, active: false },
      });
      if (draft?.id === group.id) {
        setDraft(null);
      }
      notifySuccess(`${group.name} quitado. Sus mesas siguen como mesas normales.`);
    } catch (problem) {
      // Un resultado incierto queda en «Recuperar resultado» de la página.
      if (!uncertainError(problem)) {
        notifyError(problem, 'No se pudo quitar el grupo.');
      }
    }
  }

  return (
    <Accordion
      title="Mesas divisibles"
      summary={`${area ? `${area.name} · ` : ''}${countLabel(groups.length, 'grupo', 'grupos')}`}
    >
      {dialog}
      <div className="form-stack table-groups-admin">
        <p className="muted">
          Una mesa divisible es un grupo de mesas que se dibujan juntas, por ejemplo «Mesa 3»
          formada por 3A, 3B y 3C. Cada sección se abre, se cobra y se libera por separado; «Unir
          Mesa 3» las une de un toque.
        </p>
        {groups.length === 0 && <p className="muted">Esta área no tiene mesas divisibles.</p>}
        {groups.length > 0 && (
          <ul className="table-groups-list" aria-label="Mesas divisibles del área">
            {groups.map((group) => (
              <li key={group.id} className="table-groups-item">
                <span>
                  <strong>{group.name}</strong>
                  <span className="muted">
                    {' '}
                    · {group.table_ids.map(tableName).join(', ')} ·{' '}
                    {countLabel(group.seats, 'lugar', 'lugares')}
                  </span>
                </span>
                <span className="button-row">
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={busy}
                    aria-label={`Editar ${group.name}`}
                    onClick={() =>
                      setDraft({
                        id: group.id,
                        name: group.name,
                        table_ids: group.table_ids,
                        revision: group.revision,
                      })
                    }
                  >
                    <IconLabel icon="edit">Editar</IconLabel>
                  </button>
                  <button
                    type="button"
                    className="danger-button"
                    disabled={busy}
                    aria-label={`Quitar ${group.name}`}
                    onClick={() => void remove(group)}
                  >
                    <IconLabel icon="trash">Quitar grupo</IconLabel>
                  </button>
                </span>
              </li>
            ))}
          </ul>
        )}
        <div className="button-row">
          <button
            type="button"
            className="secondary-button"
            disabled={busy || !areaID}
            onClick={() => setDraft(newGroup())}
          >
            <IconLabel icon="plus">Crear grupo</IconLabel>
          </button>
        </div>
        {draft && (
          <Form
            key={draft.id || 'nuevo'}
            disabled={busy}
            label="Guardar grupo"
            successMessage="Grupo guardado."
            onSave={() => save(draft)}
          >
            <h3>{draft.id ? `Editar ${draft.name}` : 'Nuevo grupo'}</h3>
            <div className="form-grid">
              <TextField
                label="Nombre del grupo"
                required
                value={draft.name}
                onChange={(name) => setDraft({ ...draft, name })}
              />
              <Field label="Secciones, en orden">
                <MultiSelect
                  value={draft.table_ids}
                  options={tables.map((table) => ({ value: table.id, label: table.name }))}
                  placeholder="Elige al menos dos mesas"
                  onChange={(table_ids) => setDraft({ ...draft, table_ids })}
                />
              </Field>
            </div>
            <p className="muted">
              Las secciones conservan sus nombres. Se guardan en el orden en que las eliges.
            </p>
          </Form>
        )}
      </div>
    </Accordion>
  );
}
