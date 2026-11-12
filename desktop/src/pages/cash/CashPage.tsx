import { useState } from 'react';
import { Notice, PageTitle } from '../../components/Forms';
import { permissionName } from '../../services/payments';
import { CashSettingsView } from './CashSettingsView';
import { HistoryView } from './HistoryView';
import { MovementsView } from './MovementsView';
import { OperationalView } from './OperationalView';
import type { CashSection } from './OperationalView';
import { RegisterPicker } from './RegisterPicker';
import { ReportsView } from './ReportsView';
import { useCashWorkspace } from './useCashWorkspace';

interface SectionDefinition {
  id: CashSection;
  label: string;
  permission: string;
}

const sections: SectionDefinition[] = [
  { id: 'operation', label: 'Operación', permission: 'cash.shifts.read' },
  { id: 'movements', label: 'Movimientos', permission: 'cash.shifts.read' },
  { id: 'history', label: 'Historial', permission: 'cash.history.read' },
  { id: 'reports', label: 'Reportes', permission: 'cash.shifts.read' },
  { id: 'settings', label: 'Configuración', permission: 'cash.settings.manage' },
];

// Pantalla «Caja» (etapa 7, §19): vista operativa, movimientos, historial, reportes operativos y
// configuración de caja, cada una según sus permisos. Go calcula y autoriza todo; la pantalla
// muestra lo que entrega y nunca deduce el efectivo esperado.
export function CashPage({
  can,
  userID,
  timezone,
}: {
  can: (permission: string) => boolean;
  userID: string;
  timezone: string;
}) {
  const [section, setSection] = useState<CashSection>('operation');
  const workspace = useCashWorkspace(can);
  const available = sections.filter((entry) => can(entry.permission));
  const current = available.some((entry) => entry.id === section) ? section : 'operation';
  const selected = workspace.selected;
  const ownShift = workspace.own?.shift ?? null;

  function content() {
    if (current === 'settings') {
      return <CashSettingsView catalog={workspace.catalog.data} />;
    }
    if (current === 'history') {
      return (
        <HistoryView registers={workspace.list} ownShift={ownShift} can={can} timezone={timezone} />
      );
    }
    if (!workspace.registers.data) {
      return <Notice>{workspace.registers.error || 'Consultando las cajas…'}</Notice>;
    }
    if (current === 'movements') {
      return (
        <MovementsView
          key={selected?.register.id ?? ''}
          registerID={selected?.register.id ?? ''}
          active={workspace.shift}
          ownShift={ownShift}
          can={can}
          timezone={timezone}
        />
      );
    }
    if (current === 'reports') {
      return (
        <ReportsView
          registerID={selected?.register.id ?? ''}
          shift={workspace.shift}
          can={can}
          timezone={timezone}
        />
      );
    }
    return (
      <OperationalView
        workspace={workspace}
        can={can}
        userID={userID}
        timezone={timezone}
        onNavigate={setSection}
      />
    );
  }

  return (
    <div className="cash-page form-stack">
      <PageTitle
        title="Caja"
        description="Turno de la caja, ingresos, retiros, gastos, cortes, arqueo y cierre ciego."
      />
      {!can('cash.shifts.read') && (
        <Notice>
          Consultar la caja requiere el permiso «{permissionName('cash.shifts.read')}».
        </Notice>
      )}
      <div className="catalog-tabs" role="tablist" aria-label="Secciones de caja">
        {available.map((entry) => (
          <button
            type="button"
            key={entry.id}
            role="tab"
            aria-selected={current === entry.id}
            className={current === entry.id ? 'tab-active' : ''}
            onClick={() => setSection(entry.id)}
          >
            {entry.label}
          </button>
        ))}
      </div>
      {current !== 'settings' && current !== 'history' && (
        <RegisterPicker
          registers={workspace.list}
          selectedID={selected?.register.id ?? ''}
          onSelect={workspace.selectRegister}
        />
      )}
      {content()}
    </div>
  );
}
