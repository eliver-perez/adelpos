import lightLogo from '../../../assets/brand/adel-pos-tema-claro.png';
import darkLogo from '../../../assets/brand/adel-pos-tema-oscuro.png';
import isotype from '../assets/adel-pos-isotipo.png';

// Logotipo de AD-EL POS: el completo según el tema y, en la barra compacta (debajo de 1280 px) o en
// ventanas muy angostas, el isotipo «AE» de la marca. Desde la etapa 9F (§6.1) el isotipo es el
// mismo del ícono del instalador (`src-tauri/icons/icon.png` reducido a 128 px, con su fondo blanco
// y su marco azul, que se leen igual en tema claro y oscuro); sustituye a la «A·» provisional. Los
// originales de `assets/brand/` no se modifican.
export function BrandLogo() {
  return (
    <span className="brand-logo" aria-label="AD-EL POS">
      <img className="brand-logo-light" src={lightLogo} alt="AD-EL POS" />
      <img className="brand-logo-dark" src={darkLogo} alt="" aria-hidden="true" />
      <img className="brand-logo-compact" src={isotype} alt="AD-EL POS" />
    </span>
  );
}
