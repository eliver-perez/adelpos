package operations

import (
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/payments"
)

// effectiveTaxRate devuelve la tasa de IVA que se copia a una versión confirmada: la propia del
// producto raíz de la línea (en un combo, la del combo para todo su importe) o, si el producto la
// hereda, la tasa general vigente del establecimiento. La tasa propia la expone catalog y la
// general payments; pedidos no consulta sus tablas directamente.
func (handler *service) effectiveTaxRate(productID string) (int, error) {
	own, err := catalog.ProductTaxRate(handler.ctx, handler.tx, productID)
	if err != nil {
		return 0, err
	}
	if own != nil {
		return *own, nil
	}
	return payments.CurrentTaxRate(handler.ctx, handler.tx)
}
