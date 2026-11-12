import type { Permission } from '../services/models';

export function PermissionChecklist({
  registry,
  selected,
  onChange,
}: {
  registry: Permission[];
  selected: string[];
  onChange: (keys: string[]) => void;
}) {
  return (
    <div className="permission-grid">
      {registry.map((permission) => (
        <label className="permission-option" key={permission.key}>
          <input
            type="checkbox"
            checked={selected.includes(permission.key)}
            onChange={(event) =>
              onChange(
                event.target.checked
                  ? [...selected, permission.key]
                  : selected.filter((key) => key !== permission.key),
              )
            }
          />
          <span>{permission.description}</span>
        </label>
      ))}
    </div>
  );
}
