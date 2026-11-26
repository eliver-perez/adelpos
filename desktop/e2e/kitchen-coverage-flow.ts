import { randomUUID } from 'node:crypto';
import { expect } from '@playwright/test';
import { call } from './license-flow';
import type { Driver } from './license-flow';
import { PairingRequestError, requestPairingCode } from './pairing';
import type { PairingSession } from './pairing';

// Etapa K1.1 (Fase B): ayudas de las especificaciones workflow-stagez-k11-*: estaciones, categorías e impresoras por la
// API de Go, la pantalla de cocina emparejada como lo hará la aplicación de cocina y la lectura de la cobertura.

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

export interface ServerStation {
  id: string;
  name: string;
  active: boolean;
  reception: 'printer' | 'kds' | 'both';
  printer_id: string | null;
  revision: number;
  [key: string]: unknown;
}

export interface Display {
  device_id: string;
  device_token: string;
}

// Go acepta 5 solicitudes de emparejamiento por origen cada 10 minutos (regla de la 9G); en la corrida completa estas
// llegan después de las de la 9G, la A1, la A2 y la K1: se espera a que vuelva a aceptarla, nunca se relaja el servidor.
export async function requestDisplayCodeWhenAllowed(name: string): Promise<PairingSession> {
  const deadline = Date.now() + 11 * 60_000;
  for (;;) {
    try {
      return await requestPairingCode(name, 'Android 14', 'kitchen');
    } catch (error) {
      const limited = error instanceof PairingRequestError && error.code === 'PAIRING_RATE_LIMITED';
      if (!limited || Date.now() > deadline) {
        throw error;
      }
      await new Promise((resolve) => setTimeout(resolve, 20_000));
    }
  }
}

// Una petición de la pantalla de cocina con su credencial (sin sesión), como la aplicación de cocina 0.1.0.
export async function asDisplay(display: Display, method: string, route: string, body?: unknown) {
  const response = await fetch(`${serverURL}/api/v1/${route}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'X-Device-ID': display.device_id,
      'X-Device-Token': display.device_token,
      'X-Client-Kind': 'kitchen',
      'X-Client-Version': '0.1.0',
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return { status: response.status, body: (await response.json()) as Record<string, any> };
}

export async function stations(driver: Driver) {
  return (await call(driver, 'GET', 'kitchen/stations')) as ServerStation[];
}

export async function createStation(
  driver: Driver,
  name: string,
  reception: ServerStation['reception'],
  printerID: string | null = null,
) {
  return (await call(driver, 'POST', 'kitchen/stations', {
    operation_id: randomUUID(),
    name,
    active: true,
    reception,
    printer_id: printerID,
  })) as ServerStation;
}

// Cambia una estación con todos sus datos y su revisión vigente (PATCH kitchen/stations/{id}).
export async function patchStation(driver: Driver, id: string, changes: Partial<ServerStation>) {
  const current = (await stations(driver)).find((station) => station.id === id);
  expect(current, `la estación ${id} existe`).toBeTruthy();
  return (await call(driver, 'PATCH', `kitchen/stations/${id}`, {
    ...current,
    ...changes,
    operation_id: randomUUID(),
  })) as ServerStation;
}

export async function createPrinter(driver: Driver, name: string) {
  return (await call(driver, 'POST', 'kitchen/printers', {
    operation_id: randomUUID(),
    name,
    transport: 'cups',
    destination: `cola-${name.replace(/[^A-Za-z0-9]+/g, '-')}`,
    port: 9100,
    paper_width: 80,
    copies: 1,
    cutter_option: '',
    accepts_escpos: false,
    active: true,
    fallback_id: null,
  })) as { id: string };
}

// Activa o desactiva una impresora con todos sus datos (PATCH kitchen/printers/{id}).
export async function setPrinterActive(driver: Driver, id: string, active: boolean) {
  const printers = (await call(driver, 'GET', 'kitchen/printers')) as Record<string, unknown>[];
  const current = printers.find((printer) => printer.id === id);
  expect(current, `la impresora ${id} existe`).toBeTruthy();
  await call(driver, 'PATCH', `kitchen/printers/${id}`, {
    ...current,
    active,
    operation_id: randomUUID(),
  });
}

// Una categoría activa con un producto, para que la cobertura la resuelva.
export async function categoryWithProduct(driver: Driver, name: string, code: string) {
  const category = (await call(driver, 'POST', 'catalog/categories', {
    name,
    active: true,
    color: '#d64545',
  })) as { id: string };
  const product = (await call(driver, 'POST', 'catalog/products', {
    name: `Producto de ${name}`,
    active: true,
    category_id: category.id,
    code,
    kind: 'product',
    base_price: '50',
  })) as { id: string };
  return { categoryID: category.id, productID: product.id };
}

export interface Coverage {
  unrouted: {
    station_id: string | null;
    station_name: string | null;
    station_active: boolean | null;
    effective: string;
  };
  stations: {
    id: string;
    name: string;
    active: boolean;
    reception: string;
    categories: { id: string; name: string; products: number }[];
  }[];
  categories: { id: string; route_id: string | null; station_ids: string[] }[];
  warnings: {
    code: string;
    message: string;
    device_id: string | null;
    station_id: string | null;
  }[];
}

export async function coverage(driver: Driver) {
  return (await call(driver, 'GET', 'kitchen/coverage')) as Coverage;
}

// «Productos sin ruta» por la API, con lo que está guardado como valor esperado.
export async function setUnrouted(driver: Driver, stationID: string | null) {
  const current = await coverage(driver);
  return call(driver, 'PUT', 'kitchen/unrouted', {
    operation_id: randomUUID(),
    station_id: stationID,
    expected_station_id: current.unrouted.station_id,
  });
}
