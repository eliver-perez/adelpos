import type { IdentityLimits, IdentityRaster } from '../../../services/documentIdentity';

// Escala de la tira de papel en pantalla: un punto impreso ocupa 0.6 píxeles CSS, de modo que la
// tira de 80 mm se ve más ancha que la de 58 mm en la misma proporción que el papel real.
const dotScale = 0.6;

// Cómo saldrá el logotipo en papel de 58 y 80 mm: las imágenes son exactamente los puntos que
// convirtió Go (incluido el margen blanco de la alineación). El papel se ve blanco también en el
// tema oscuro, como un ticket real.
export function PaperPreview({
  rasters,
  limits,
  hasLogo,
  status,
}: {
  rasters: IdentityRaster[];
  limits: IdentityLimits;
  hasLogo: boolean;
  status: string;
}) {
  return (
    <div className="form-stack">
      <p className="muted text-sm" aria-live="polite">
        {status}
      </p>
      <div className="flex flex-wrap items-start gap-5">
        {limits.paper_widths.map((paper) => {
          const raster = rasters.find((candidate) => candidate.paper_width === paper.paper_width);
          return (
            <figure
              key={paper.paper_width}
              className="m-0 grid min-w-0 max-w-full gap-2"
              aria-label={`Papel de ${paper.paper_width} mm`}
            >
              <figcaption className="text-sm font-semibold">
                Papel de {paper.paper_width} mm · {paper.width_dots} puntos
              </figcaption>
              <div
                className="max-w-full rounded-md border border-[#d7dce3] bg-white px-2 py-4 shadow-sm"
                style={{ width: `${paper.width_dots * dotScale + 18}px` }}
              >
                {raster?.data_uri ? (
                  <img
                    src={raster.data_uri}
                    alt={`Logotipo convertido para papel de ${paper.paper_width} mm`}
                    className="block h-auto w-full [image-rendering:pixelated]"
                    width={raster.width_dots}
                    height={raster.height_dots}
                  />
                ) : (
                  <p className="m-0 py-6 text-center text-xs text-[#5b6675]">
                    {hasLogo ? 'Vista previa no disponible' : 'Sin logotipo: no se imprime nada'}
                  </p>
                )}
              </div>
              {raster && (
                <span className="muted text-xs">
                  {raster.width_dots} × {raster.height_dots} puntos
                </span>
              )}
            </figure>
          );
        })}
      </div>
    </div>
  );
}
