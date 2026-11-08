import { useEffect, useId, useRef, useState } from 'react';
import type { ReactNode, SyntheticEvent } from 'react';
import { flushSync } from 'react-dom';

interface AccordionProps {
  title: ReactNode;
  summary?: ReactNode;
  children: ReactNode;
  defaultOpen?: boolean;
  hasError?: boolean;
  className?: string;
}

export function Accordion({
  title,
  summary,
  children,
  defaultOpen = false,
  hasError = false,
  className = '',
}: AccordionProps) {
  const [expanded, setExpanded] = useState(defaultOpen || hasError);
  const identifier = useId();
  const headingID = `${identifier}-heading`;
  const panelID = `${identifier}-panel`;
  const section = useRef<HTMLElement>(null);

  useEffect(() => {
    const element = section.current;
    const reveal = () => setExpanded(true);
    element?.addEventListener('form-validation-error', reveal);
    return () => element?.removeEventListener('form-validation-error', reveal);
  }, []);

  useEffect(() => {
    if (hasError) {
      setExpanded(true);
    }
  }, [hasError]);

  function revealInvalidField(event: SyntheticEvent) {
    if (expanded) {
      return;
    }
    // Los campos permanecen montados. Una validación nativa debe revelar el primer error
    // antes de que el navegador intente enfocar un control dentro del panel cerrado.
    event.preventDefault();
    flushSync(() => setExpanded(true));
    if (event.target instanceof HTMLElement) {
      event.target.focus();
    }
  }

  return (
    <section
      ref={section}
      className={`accordion ${className}`}
      data-expanded={expanded}
      onInvalidCapture={revealInvalidField}
    >
      <h3 className="accordion-heading">
        <button
          type="button"
          id={headingID}
          className="accordion-trigger"
          aria-expanded={expanded}
          aria-controls={panelID}
          onClick={() => setExpanded((current) => !current)}
        >
          <span className="accordion-label">
            <span className="accordion-title">{title}</span>
            {summary && <span className="accordion-summary">{summary}</span>}
          </span>
          <svg
            className="accordion-chevron"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.8"
            aria-hidden="true"
          >
            <path d="m6 9 6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
      </h3>
      <div
        id={panelID}
        role="region"
        aria-labelledby={headingID}
        aria-hidden={!expanded}
        inert={!expanded}
        className="accordion-panel"
      >
        <div className="accordion-clip">
          <div className="accordion-content">{children}</div>
        </div>
      </div>
    </section>
  );
}
