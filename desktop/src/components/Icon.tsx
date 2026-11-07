type IconName = 'server' | 'database' | 'monitor' | 'refresh' | 'sun' | 'moon' | 'check' | 'arrow' | 'activity';
const paths: Record<IconName, React.ReactNode> = {
  server: <><rect x="4" y="3" width="16" height="7" rx="2"/><rect x="4" y="14" width="16" height="7" rx="2"/><path d="M8 6.5h.01M8 17.5h.01M12 6.5h5M12 17.5h5"/></>,
  database: <><ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 4 16 4 16 0V5M4 12c0 4 16 4 16 0"/></>,
  monitor: <><rect x="3" y="3" width="18" height="13" rx="2"/><path d="M8 21h8M12 16v5"/></>,
  refresh: <><path d="M20 7v5h-5M4 17v-5h5M5.6 7a7 7 0 0 1 11.5-2L20 8M4 16l2.9 3A7 7 0 0 0 18.4 17"/></>,
  sun: <><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1 1M18 18l1 1M5 19l1-1M18 6l1-1"/></>,
  moon: <path d="M20.8 13.4A9 9 0 0 1 10.6 3.2 9 9 0 1 0 20.8 13.4Z"/>,
  check: <path d="m5 12 4 4L19 6"/>,
  arrow: <path d="M4 12h16m-5-5 5 5-5 5"/>,
  activity: <><path d="M3 12h4l3-8 4 16 3-8h4"/></>,
};
export function Icon({ name, className = '' }: { name: IconName; className?: string }) {
  return <svg className={className} width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>;
}
