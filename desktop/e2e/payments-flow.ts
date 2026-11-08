import { expect } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';

// Recorridos de cobro de la etapa 6 contra Go y PostgreSQL reales. Todas las cifras que se
// verifican provienen de Go; la interfaz solo las muestra.

async function typeOnKeypad(keypad: Locator, digits: string) {
  for (const digit of digits) {
    await keypad.getByRole('button', { name: digit, exact: true }).click();
  }
}

function checkoutStatus(page: Page) {
  return page.locator('.checkout-status');
}

// Configura la impresora de tickets en Cobros → Políticas con el destino controlado de la
// aceptación (sin hardware). El ticket queda en la cola durable con su estado real.
export async function configureReceiptPrinter(page: Page) {
  await page.getByRole('button', { name: 'Cobros', exact: true }).click();
  await page.getByRole('button', { name: /Políticas de cobro/ }).click();
  await page
    .getByRole('combobox', { name: /Impresora de tickets/ })
    .selectOption({ label: 'Destino controlado E2E' });
  await page.getByRole('button', { name: 'Guardar políticas', exact: true }).click();
  await expect(page.getByRole('combobox', { name: /Impresora de tickets/ })).toHaveValue(/.+/);
  await page.getByRole('button', { name: 'Restaurante', exact: true }).click();
}

// Cuenta de salón con un producto de $150: apertura de turno, abono en efectivo con cambio,
// precuenta, descuento general, pago con tarjeta que liquida y ticket en la cola.
export async function checkDineInPayment(page: Page) {
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await bill.getByRole('button', { name: 'Cobrar $150.00', exact: true }).click();

  const shift = page.getByRole('region', { name: 'Turno de caja' });
  await expect(shift).toBeVisible();
  await typeOnKeypad(
    shift.getByRole('group', { name: 'Teclado numérico del fondo inicial' }),
    '500',
  );
  await expect(shift.getByLabel('Fondo inicial', { exact: true })).toHaveValue('500');
  await shift.getByRole('button', { name: 'Abrir turno con fondo de $500', exact: true }).click();

  // Abono de $100 en efectivo con $200 recibidos: Go calcula $100 de cambio.
  await page
    .getByRole('group', { name: 'Tipo de pago' })
    .getByRole('button', { name: 'Pago parcial', exact: true })
    .click();
  const methods = page.getByRole('group', { name: 'Métodos de pago disponibles' });
  await methods.getByRole('button', { name: 'Efectivo', exact: true }).click();
  await expect(methods.getByRole('button', { name: 'Efectivo', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(page.locator('.checkout-workspace input[type="checkbox"]')).toHaveCount(0);
  const keypad = page.getByRole('group', { name: 'Teclado numérico de cobro' });
  await page.getByLabel('Importe del abono', { exact: true }).click();
  await typeOnKeypad(keypad, '100');
  await expect(page.getByLabel('Importe del abono', { exact: true })).toHaveValue('100');
  await page.getByLabel('Efectivo recibido', { exact: true }).click();
  await page
    .getByRole('group', { name: 'Importes rápidos' })
    .getByRole('button', { name: '$200', exact: true })
    .click();
  await expect(page.locator('.checkout-figure-emphasis')).toContainText('$100.00');
  const print = page.getByRole('button', { name: 'Imprimir ticket', exact: true });
  await expect(print).toHaveAttribute('aria-pressed', 'true');
  const register = page.locator('.checkout-register');
  await expect(register).toHaveText(/Registrar \$100\.00 · Efectivo/);
  const bounds = await register.boundingBox();
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  await register.click();
  await expect(page.getByText('Pago registrado: $100.00 con Efectivo')).toBeVisible();
  await expect(page.getByText(/Entrega cambio \$100\.00/)).toBeVisible();
  await expect(page.getByText(/Saldo pendiente \$50\.00/).first()).toBeVisible();
  await expect(page.getByText(/cola de impresión|Ticket enviado/i).first()).toBeVisible();

  // Precuenta: no registra venta; muestra IVA, pagos registrados y saldo.
  const toolbar = page.getByRole('group', { name: 'Acciones del cobro' });
  await toolbar.getByRole('button', { name: 'Precuenta', exact: true }).click();
  const preview = page.getByLabel('Vista previa de la precuenta');
  await expect(preview).toContainText('PRECUENTA');
  await expect(preview).toContainText('IVA 16 %');
  await expect(preview).toContainText('PAGOS REGISTRADOS');
  await expect(preview).toContainText('$50.00');
  await expect(preview).toContainText('CFDI');
  const precheck = page.getByRole('region', { name: 'Precuenta' });
  await precheck.getByRole('button', { name: 'Imprimir precuenta', exact: true }).click();
  // El destino de aceptación no tiene hardware: el trabajo queda en la cola con su estado real.
  await expect(precheck.getByRole('status')).toContainText(/Precuenta.*(cola|Cola) de impresión/);
  await precheck.getByRole('button', { name: 'Volver', exact: true }).click();

  // Descuento general del 10 % dentro del límite del administrador: sin autorización adicional.
  await toolbar.getByRole('button', { name: 'Descuentos y cargos', exact: true }).click();
  const adjustments = page.getByRole('region', { name: 'Descuentos y cargos' });
  await adjustments.getByLabel(/Porcentaje de descuento/).fill('10');
  await adjustments.getByLabel(/^Motivo/).fill('Cliente frecuente');
  await expect(adjustments.getByText('Autorización de supervisor')).toHaveCount(0);
  await adjustments.getByRole('button', { name: /^Aplicar descuento \$15\.00/ }).click();
  await expect(adjustments.getByText('Descuento registrado.')).toBeVisible();
  await adjustments.getByRole('button', { name: 'Volver', exact: true }).click();

  // Pago mixto sucesivo: el saldo restante de $35 con tarjeta liquida y cierra la cuenta.
  await methods.getByRole('button', { name: 'Tarjeta de débito', exact: true }).click();
  await expect(register).toHaveText(/Registrar \$35\.00 · Tarjeta de débito/);
  await register.click();
  await expect(page.getByText('Pago registrado: $35.00 con Tarjeta de débito')).toBeVisible();
  await expect(page.getByText(/La cuenta se cerró/)).toBeVisible();
  await expect(checkoutStatus(page)).toHaveText(/Cuenta liquidada|Impresión pendiente/);
  const documents = page.getByRole('region', { name: 'Documentos de la cuenta' });
  await expect(documents).toContainText('Ticket de pago');
  await expect(documents).toContainText('Precuenta');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}

// Pedido para llevar con saldo: «Marcar como entregado» abre el cobro real con la entrega
// activada y el pago que liquida registra también la entrega.
export async function checkTakeawayDelivery(page: Page) {
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await page
    .getByRole('button', { name: 'Capturar productos en esta cuenta', exact: true })
    .click();
  await page.getByRole('button', { name: /Filete de prueba/ }).click();
  await page.getByRole('button', { name: 'Guardar en borrador', exact: true }).click();
  const confirm = page.getByRole('button', { name: 'Confirmar pedido', exact: true });
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect(bill).toContainText('$150.00');
  await expect(bill).toContainText('Entrega: no entregado');
  await bill.getByRole('button', { name: 'Marcar como entregado', exact: true }).click();
  const deliver = page
    .locator('.checkout-workspace')
    .getByRole('button', { name: 'Marcar como entregado', exact: true });
  await expect(deliver).toHaveAttribute('aria-pressed', 'true');
  await page
    .getByRole('group', { name: 'Métodos de pago disponibles' })
    .getByRole('button', { name: 'Efectivo', exact: true })
    .click();
  await page
    .getByRole('group', { name: 'Importes rápidos' })
    .getByRole('button', { name: 'Exacto', exact: true })
    .click();
  const register = page.locator('.checkout-register');
  await expect(register).toHaveText(/Registrar \$150\.00 · Efectivo/);
  await register.click();
  await expect(page.getByText('Pago registrado: $150.00 con Efectivo')).toBeVisible();
  await expect(page.getByText('Entrega realizada.').first()).toBeVisible();
  await page.getByRole('button', { name: 'Volver a la cuenta', exact: true }).first().click();
}
