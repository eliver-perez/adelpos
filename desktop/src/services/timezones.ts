// Zonas horarias IANA para el selector de configuración (etapa 6.1 §2.7). La lista sale del
// navegador (Intl.supportedValuesOf); Go confirma con time.LoadLocation que la zona exista.

// Zonas de México, en el orden en que se ofrecen primero.
export const mexicoTimezones = [
  'America/Monterrey',
  'America/Mexico_City',
  'America/Tijuana',
  'America/Hermosillo',
  'America/Chihuahua',
  'America/Ciudad_Juarez',
  'America/Matamoros',
  'America/Mazatlan',
  'America/Bahia_Banderas',
  'America/Merida',
  'America/Cancun',
  'America/Ojinaga',
];

// Respaldo si el motor no ofrece Intl.supportedValuesOf (WebKit anterior a 15.4).
const fallbackTimezones = [
  'America/Bogota',
  'America/Chicago',
  'America/Denver',
  'America/Guatemala',
  'America/Lima',
  'America/Los_Angeles',
  'America/New_York',
  'America/Phoenix',
  'America/Santiago',
  'America/Argentina/Buenos_Aires',
  'Europe/Madrid',
  'UTC',
];

function supportedTimezones(): string[] {
  try {
    const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] };
    if (typeof intl.supportedValuesOf === 'function') {
      return intl.supportedValuesOf('timeZone');
    }
  } catch {
    // Se usa el respaldo.
  }
  return [];
}

// Lista completa sin repetidos: México primero, luego el resto en orden alfabético. La zona ya
// guardada siempre aparece, aunque el motor no la incluya.
export function timezoneOptions(current = ''): string[] {
  const reported = supportedTimezones();
  const others = (reported.length > 0 ? reported : fallbackTimezones)
    .filter((zone) => !mexicoTimezones.includes(zone))
    .sort((first, second) => first.localeCompare(second));
  const result = [...mexicoTimezones, ...others];
  if (current && !result.includes(current)) {
    result.unshift(current);
  }
  return result;
}

// «America/Mexico_City» se muestra como «America/Mexico City».
export function timezoneLabel(zone: string): string {
  return zone.replaceAll('_', ' ');
}

export interface TimezoneGroup {
  label: string;
  options: { value: string; label: string; keywords?: string }[];
}

// Grupos del selector de zona horaria (etapa 9C): México primero y después las demás. La búsqueda
// del selector compara el texto visible sin acentos ni guiones bajos; las de México también se
// encuentran escribiendo «México».
export function timezoneGroups(current = ''): TimezoneGroup[] {
  const zones = timezoneOptions(current);
  const mexico = zones.filter((zone) => mexicoTimezones.includes(zone));
  const others = zones.filter((zone) => !mexicoTimezones.includes(zone));
  return [
    {
      label: 'México',
      options: mexico.map((zone) => ({
        value: zone,
        label: timezoneLabel(zone),
        keywords: 'México',
      })),
    },
    {
      label: 'Otras zonas',
      options: others.map((zone) => ({ value: zone, label: timezoneLabel(zone) })),
    },
  ];
}
