import { useEffect, useRef, useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Form, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryItem, Page } from '../../services/inventory';
import { createSupplier, saveSupplier } from '../../services/inventoryPurchasing';
import type { Supplier, SupplierDetail, SupplierInput } from '../../services/inventoryPurchasing';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { ActiveToggle, ItemPicker, Pagination } from './InventoryFields';
import { purchasingPermissionName } from './purchasingFormat';
import type { PurchasingNavigation } from './purchasingNavigation';
import { SupplierHistorySection, SupplierItemsSection } from './SupplierSections';
import { Select } from '../../components/Select';

const pageSize = 25;

interface SupplierFilter {
  search: string;
  active: string;
  // Modalidad: '' (todas), formal o counter (Compras de mostrador).
  kind: string;
  item: InventoryItem | null;
  page: number;
}

function supplierQuery(filter: SupplierFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.search.trim() !== '') {
    query.search = filter.search.trim();
  }
  if (filter.active !== '') {
    query.active = filter.active;
  }
  if (filter.kind !== '') {
    query.kind = filter.kind;
  }
  if (filter.item) {
    query.item_id = filter.item.id;
  }
  return query;
}

interface SupplierFormState {
  tradeName: string;
  legalName: string;
  rfc: string;
  phone: string;
  phoneSecondary: string;
  mobilePhone: string;
  website: string;
  email: string;
  address: string;
  contact: string;
  notes: string;
  active: boolean;
}

function initialForm(supplier: SupplierDetail | null): SupplierFormState {
  return {
    tradeName: supplier?.trade_name ?? '',
    legalName: supplier?.legal_name ?? '',
    rfc: supplier?.rfc ?? '',
    phone: supplier?.phone ?? '',
    phoneSecondary: supplier?.phone_secondary ?? '',
    mobilePhone: supplier?.mobile_phone ?? '',
    website: supplier?.website ?? '',
    email: supplier?.email ?? '',
    address: supplier?.address ?? '',
    contact: supplier?.contact ?? '',
    notes: supplier?.notes ?? '',
    active: supplier?.active ?? true,
  };
}

// Cuerpo del proveedor para Go con los textos tal como se capturaron. El RFC vacío es «sin RFC»;
// Go lo valida (formato del SAT y fecha) y lo guarda en mayúsculas. Al crear, «activo» se omite.
// Segundo teléfono, celular y página web son opcionales; Go valida su formato. «Compras de
// mostrador» nunca lleva RFC ni razón social.
export function supplierInput(
  form: SupplierFormState,
  supplier: SupplierDetail | null,
): SupplierInput {
  const counter = supplier?.kind === 'counter';
  const input: SupplierInput = {
    trade_name: form.tradeName,
    legal_name: counter ? '' : form.legalName,
    rfc: counter || form.rfc.trim() === '' ? null : form.rfc,
    phone: form.phone,
    phone_secondary: form.phoneSecondary,
    mobile_phone: form.mobilePhone,
    website: form.website,
    email: form.email,
    address: form.address,
    contact: form.contact,
    notes: form.notes,
  };
  if (supplier) {
    input.revision = supplier.revision;
    input.active = form.active;
  }
  return input;
}

function SupplierFilters({
  filter,
  onChange,
}: {
  filter: SupplierFilter;
  onChange: (filter: SupplierFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de proveedores">
      <Field label="Buscar proveedor">
        <input
          type="search"
          maxLength={100}
          value={filter.search}
          placeholder="Nombre, razón social, RFC, contacto, teléfonos, web o correo"
          onChange={(event) => onChange({ ...filter, search: event.target.value, page: 1 })}
        />
      </Field>
      <Field label="Modalidad">
        <Select
          value={filter.kind}
          options={[
            { value: '', label: 'Todas' },
            { value: 'formal', label: 'Proveedores formales' },
            { value: 'counter', label: 'Compras de mostrador' },
          ]}
          onChange={(kind) => onChange({ ...filter, kind, page: 1 })}
        />
      </Field>
      <Field label="Estado">
        <Select
          value={filter.active}
          options={[
            { value: '', label: 'Todos' },
            { value: 'true', label: 'Activos' },
            { value: 'false', label: 'Inactivos' },
          ]}
          onChange={(active) => onChange({ ...filter, active, page: 1 })}
        />
      </Field>
      <ItemPicker
        label="Insumo surtido"
        value={filter.item}
        activeOnly={false}
        emptyLabel="Cualquier insumo"
        onChange={(item) => onChange({ ...filter, item, page: 1 })}
      />
    </section>
  );
}

function contactSummary(supplier: Supplier) {
  const parts = [supplier.contact, supplier.phone, supplier.mobile_phone, supplier.email].filter(
    (part) => part.trim() !== '',
  );
  return parts.length > 0 ? parts.join(' · ') : '—';
}

function SuppliersTable({
  page,
  editable,
  timezone,
  onOpen,
}: {
  page: Page<Supplier>;
  editable: boolean;
  timezone: string;
  onOpen: (supplier: Supplier) => void;
}) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Proveedor</th>
            <th scope="col">RFC</th>
            <th scope="col">Contacto</th>
            <th scope="col">Insumos</th>
            <th scope="col">Última recepción</th>
            <th scope="col">Estado</th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((supplier) => (
            <tr key={supplier.id}>
              <td className="inventory-cell-name">
                <strong>{supplier.trade_name}</strong>
                {supplier.legal_name && (
                  <span className="muted text-xs inventory-cell-detail">{supplier.legal_name}</span>
                )}
                {supplier.kind === 'counter' && (
                  <span className="tag inventory-inline-tag">Compras de mostrador</span>
                )}
              </td>
              <td>{supplier.rfc ?? '—'}</td>
              <td>{contactSummary(supplier)}</td>
              <td>{countLabel(supplier.item_count, 'insumo', 'insumos')}</td>
              <td>
                {supplier.last_receipt_at
                  ? serverDateTime(supplier.last_receipt_at, timezone)
                  : 'Sin recepciones'}
              </td>
              <td>
                <span className={supplier.active ? 'tag tag-good' : 'tag tag-muted'}>
                  {supplier.active ? 'Activo' : 'Inactivo'}
                </span>
              </td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`${editable ? 'Editar' : 'Ver'} el proveedor ${supplier.trade_name}`}
                  onClick={() => onOpen(supplier)}
                >
                  {editable ? 'Editar' : 'Ver'}
                </button>
              </td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={7} className="muted">
                No hay proveedores con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function SupplierForm({
  supplier,
  editable,
  execute,
  busy,
  onSaved,
  onDirtyChange,
}: {
  supplier: SupplierDetail | null;
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  onSaved: (supplier: SupplierDetail) => void;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const [initial] = useState(() => initialForm(supplier));
  const [form, setForm] = useState(initial);
  const dirty = JSON.stringify(form) !== JSON.stringify(initial);
  const counter = supplier?.kind === 'counter';

  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);

  function update<K extends keyof SupplierFormState>(key: K, value: SupplierFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  const deactivationMessage = supplier?.has_purchases
    ? `Al guardar, «${supplier.trade_name}» no admitirá órdenes nuevas ni recepciones directas. Sus compras, recepciones y devoluciones se conservan; la mercancía de una orden ya emitida todavía se puede recibir.`
    : `Al guardar, «${supplier?.trade_name ?? ''}» dejará de ofrecerse en órdenes y recepciones. Puedes volver a activarlo después.`;

  return (
    <Form
      label={supplier ? 'Guardar proveedor' : 'Crear proveedor'}
      successMessage={
        supplier
          ? 'Proveedor guardado.'
          : 'Proveedor creado. Ya puedes agregar los insumos que surte.'
      }
      disabled={busy || !editable}
      onSave={async () => {
        const input = supplierInput(form, supplier);
        const saved = supplier
          ? await saveSupplier(execute, supplier.id, input)
          : await createSupplier(execute, input);
        onDirtyChange(false);
        onSaved(saved);
      }}
    >
      {counter && (
        <p className="muted text-sm">
          Proveedor especial para compras ocasionales en mercados, centrales de abastos,
          supermercados o comercios sin proveedor registrado. Solo se usa en recepciones directas,
          no lleva RFC ni razón social y no registra insumos surtidos: el comercio concreto se anota
          en cada recepción.
        </p>
      )}
      <div className="form-grid">
        <Field label="Nombre comercial" hint="Único entre proveedores (sin distinguir mayúsculas).">
          <input
            required
            maxLength={100}
            value={form.tradeName}
            onChange={(event) => update('tradeName', event.target.value)}
          />
        </Field>
        {!counter && (
          <Field label="Razón social" hint="Opcional.">
            <input
              maxLength={200}
              value={form.legalName}
              onChange={(event) => update('legalName', event.target.value)}
            />
          </Field>
        )}
        {!counter && (
          <Field
            label="RFC"
            hint="Opcional: 12 caracteres (persona moral) o 13 (persona física). Al guardar se revisan el formato y la fecha."
          >
            <input
              maxLength={13}
              autoComplete="off"
              autoCapitalize="characters"
              spellCheck={false}
              value={form.rfc}
              onChange={(event) => update('rfc', event.target.value)}
            />
          </Field>
        )}
        <Field label="Contacto" hint="Persona con quien se trata.">
          <input
            maxLength={100}
            value={form.contact}
            onChange={(event) => update('contact', event.target.value)}
          />
        </Field>
        <Field label="Teléfono">
          <input
            type="tel"
            maxLength={40}
            autoComplete="off"
            value={form.phone}
            onChange={(event) => update('phone', event.target.value)}
          />
        </Field>
        <Field
          label="Segundo teléfono"
          hint="Opcional; de 7 a 20 dígitos, con extensión si la hay."
        >
          <input
            type="tel"
            maxLength={40}
            autoComplete="off"
            value={form.phoneSecondary}
            onChange={(event) => update('phoneSecondary', event.target.value)}
          />
        </Field>
        <Field label="Teléfono celular" hint="Opcional; de 7 a 20 dígitos.">
          <input
            type="tel"
            maxLength={40}
            autoComplete="off"
            value={form.mobilePhone}
            onChange={(event) => update('mobilePhone', event.target.value)}
          />
        </Field>
        <Field
          label="Página web"
          hint="Opcional; por ejemplo proveedor.com.mx. Se muestra como texto."
        >
          <input
            inputMode="url"
            maxLength={200}
            autoComplete="off"
            autoCapitalize="off"
            spellCheck={false}
            value={form.website}
            onChange={(event) => update('website', event.target.value)}
          />
        </Field>
        <Field label="Correo electrónico">
          <input
            type="email"
            maxLength={120}
            autoComplete="off"
            value={form.email}
            onChange={(event) => update('email', event.target.value)}
          />
        </Field>
      </div>
      <Field label="Domicilio" hint="Opcional; admite varias líneas.">
        <textarea
          rows={2}
          maxLength={500}
          value={form.address}
          onChange={(event) => update('address', event.target.value)}
        />
      </Field>
      <Field label="Observaciones del proveedor">
        <textarea
          rows={2}
          maxLength={2000}
          value={form.notes}
          onChange={(event) => update('notes', event.target.value)}
        />
      </Field>
      {supplier && (
        <div className="button-row">
          <ActiveToggle
            label="Proveedor activo"
            pressed={form.active}
            onChange={(value) => update('active', value)}
            confirmDeactivation={supplier.active}
            title="Desactivar proveedor"
            message={deactivationMessage}
          />
          {supplier.has_purchases && (
            <span className="muted text-xs">
              Con compras registradas: no se borra, solo se desactiva.
            </span>
          )}
        </div>
      )}
    </Form>
  );
}

// Panel de alta y edición de un proveedor: datos generales y, una vez creado, los insumos que
// surte y su historial de compras. Los cambios de otra terminal recargan el proveedor.
function SupplierEditorPanel({
  initialSupplierID,
  editable,
  execute,
  busy,
  timezone,
  navigation,
  onClose,
}: {
  initialSupplierID: string;
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  navigation: PurchasingNavigation;
  onClose: () => void;
}) {
  const [supplierID, setSupplierID] = useState(initialSupplierID);
  const [snapshot, setSnapshot] = useState<SupplierDetail | null>(null);
  const dirty = useRef(false);
  const { confirm, dialog } = useConfirmDialog();
  const resource = useResource<SupplierDetail>('inventory_supplier', {
    id: supplierID,
    enabled: supplierID !== '',
    reloadEvent: inventoryChangeEvent,
  });

  useEffect(() => {
    if (resource.data) {
      setSnapshot(resource.data);
    }
  }, [resource.data]);

  const supplier = supplierID === '' ? null : snapshot?.id === supplierID ? snapshot : null;

  async function close() {
    if (dirty.current) {
      const discard = await confirm({
        title: 'Cambios sin guardar',
        message: 'Este proveedor tiene cambios que aún no se guardan. ¿Quieres descartarlos?',
        confirmLabel: 'Descartar cambios',
        cancelLabel: 'Seguir editando',
        tone: 'danger',
      });
      if (!discard) {
        return;
      }
    }
    onClose();
  }

  return (
    <section
      className="panel form-stack"
      aria-label={supplier ? `Proveedor ${supplier.trade_name}` : 'Nuevo proveedor'}
    >
      <div className="section-heading">
        <div>
          <h2>
            {supplier ? supplier.trade_name : supplierID === '' ? 'Nuevo proveedor' : 'Proveedor'}
          </h2>
          <p className="muted text-xs">
            {supplier
              ? `${supplier.active ? 'Activo' : 'Inactivo'} · ${countLabel(supplier.item_count, 'insumo surtido', 'insumos surtidos')}${supplier.updated_by_name ? ` · última edición: ${supplier.updated_by_name}` : ''}`
              : 'Se guardará al confirmar'}
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={() => void close()}>
          Volver al listado
        </button>
      </div>
      {!editable && (
        <Notice>
          Consulta disponible. Editar proveedores requiere el permiso «
          {purchasingPermissionName('inventory.suppliers.manage')}».
        </Notice>
      )}
      {supplierID !== '' && !supplier && (
        <Notice>{resource.error || 'Consultando el proveedor…'}</Notice>
      )}
      {(supplierID === '' || supplier) && (
        <SupplierForm
          key={supplier ? `${supplier.id}:${supplier.revision}` : 'new'}
          supplier={supplier}
          editable={editable}
          execute={execute}
          busy={busy}
          onDirtyChange={(value) => {
            dirty.current = value;
          }}
          onSaved={(saved) => {
            setSnapshot(saved);
            setSupplierID(saved.id);
          }}
        />
      )}
      {supplier && supplier.kind !== 'counter' && (
        <SupplierItemsSection
          supplier={supplier}
          editable={editable}
          execute={execute}
          busy={busy}
          timezone={timezone}
          onChanged={setSnapshot}
        />
      )}
      {supplier && (
        <SupplierHistorySection supplier={supplier} timezone={timezone} navigation={navigation} />
      )}
      {dialog}
    </section>
  );
}

// Pestaña «Proveedores»: listado con búsqueda, estado e insumo surtido; alta, edición,
// desactivación (no hay borrado: un proveedor con compras conserva su historial), insumos que
// surte e historial de compras. Editar exige inventory.suppliers.manage; Go lo vuelve a comprobar.
export function SuppliersView({
  can,
  execute,
  busy,
  timezone,
  initialSupplierID,
  navigation,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  initialSupplierID: string | null;
  navigation: PurchasingNavigation;
}) {
  const [filter, setFilter] = useState<SupplierFilter>({
    search: '',
    active: 'true',
    kind: '',
    item: null,
    page: 1,
  });
  // null: listado; '': proveedor nuevo; id: proveedor abierto.
  const [editing, setEditing] = useState<string | null>(initialSupplierID);
  const list = useResource<Page<Supplier>>('inventory_suppliers', {
    query: supplierQuery(filter),
    reloadEvent: inventoryChangeEvent,
  });
  const editable = can('inventory.suppliers.manage');

  if (editing !== null) {
    return (
      <SupplierEditorPanel
        initialSupplierID={editing}
        editable={editable}
        execute={execute}
        busy={busy}
        timezone={timezone}
        navigation={navigation}
        onClose={() => setEditing(null)}
      />
    );
  }

  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Catálogo de proveedores con los insumos que surten y su historial de compras. Un proveedor
          con compras no se borra: se desactiva.
        </p>
        {editable && (
          <button type="button" className="primary-button" onClick={() => setEditing('')}>
            <IconLabel icon="plus">Nuevo proveedor</IconLabel>
          </button>
        )}
      </div>
      <SupplierFilters filter={filter} onChange={setFilter} />
      <Notice>{list.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando proveedores…</p>}
      {list.data && (
        <section className="panel">
          <SuppliersTable
            page={list.data}
            editable={editable}
            timezone={timezone}
            onOpen={(supplier) => setEditing(supplier.id)}
          />
          <Pagination
            page={list.data.page}
            pageSize={list.data.page_size}
            total={list.data.total}
            singular="proveedor"
            plural="proveedores"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </section>
      )}
    </div>
  );
}
