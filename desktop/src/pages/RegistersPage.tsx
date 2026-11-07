import { useState } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { Register } from '../services/models';

function RegisterEditor({
  register,
  reload,
  close,
}: {
  register: Register | null;
  reload: () => Promise<void>;
  close: () => void;
}) {
  const [name, setName] = useState(register?.name ?? '');
  const [description, setDescription] = useState(register?.description ?? '');
  const [active, setActive] = useState(register?.active ?? true);
  return (
    <section className="surface panel">
      <div className="section-heading">
        <h2>{register ? 'Editar caja física' : 'Nueva caja física'}</h2>
        <button className="secondary-button" onClick={close}>
          Cerrar editor
        </button>
      </div>
      <Form
        label="Guardar caja"
        onSave={async () => {
          await api(register ? 'update_register' : 'create_register', {
            id: register?.id,
            body: { name, description, active },
          });
          await reload();
        }}
      >
        <div className="form-grid">
          <Field label="Nombre">
            <input
              required
              maxLength={80}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <Field label="Descripción">
            <input
              maxLength={500}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </Field>
        </div>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={active}
            onChange={(event) => setActive(event.target.checked)}
          />
          Caja activa
        </label>
      </Form>
    </section>
  );
}

export function RegistersPage({ can }: { can: (key: string) => boolean }) {
  const resource = useResource<Register[]>('registers');
  const [editing, setEditing] = useState<Register | null | undefined>(undefined);
  return (
    <>
      <PageTitle
        title="Cajas físicas"
        description="Configuración de los puntos físicos de caja. Sin turnos ni movimientos económicos."
      >
        {can('registers.manage') && (
          <button className="primary-button" onClick={() => setEditing(null)}>
            Crear caja
          </button>
        )}
      </PageTitle>
      <Notice>{resource.error}</Notice>
      {editing !== undefined && (
        <RegisterEditor
          key={editing?.id ?? 'new'}
          register={editing}
          reload={resource.reload}
          close={() => setEditing(undefined)}
        />
      )}
      <div className="surface table-container">
        <table>
          <thead>
            <tr>
              <th>Caja</th>
              <th>Descripción</th>
              <th>Estado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            {resource.data?.map((register) => (
              <tr key={register.id}>
                <td>
                  <strong>{register.name}</strong>
                </td>
                <td>{register.description || '—'}</td>
                <td>{register.active ? 'Activa' : 'Inactiva'}</td>
                <td>
                  {can('registers.manage') && (
                    <button className="secondary-button" onClick={() => setEditing(register)}>
                      Editar
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
