import lightLogo from '../../../assets/brand/adel-pos-tema-claro.png';
import darkLogo from '../../../assets/brand/adel-pos-tema-oscuro.png';

export function BrandLogo() {
  return (
    <span className="brand-logo" aria-label="AD-EL POS">
      <img className="brand-logo-light" src={lightLogo} alt="AD-EL POS" />
      <img className="brand-logo-dark" src={darkLogo} alt="" aria-hidden="true" />
      <span className="brand-mark brand-logo-compact" aria-hidden="true">
        A<span>·</span>
      </span>
    </span>
  );
}
