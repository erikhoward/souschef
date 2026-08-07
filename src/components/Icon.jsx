const paths = {
  arrow: <path d="M5 12h14m-6-6 6 6-6 6" />,
  archive: <><path d="M4 7h16v12H4z" /><path d="M3 4h18v3H3zM9 11h6" /></>,
  book: <><path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H11v16H6.5A2.5 2.5 0 0 0 4 21.5z" /><path d="M20 5.5A2.5 2.5 0 0 0 17.5 3H13v16h4.5a2.5 2.5 0 0 1 2.5 2.5z" /></>,
  check: <path d="m5 12 4 4L19 6" />,
  chevron: <path d="m6 9 6 6 6-6" />,
  clapper: <><path d="M4 7h16v13H4z" /><path d="M4 7 7 3h13l-3 4M4 12h16M8 3l3 4m4-4 3 4" /></>,
  clock: <><circle cx="12" cy="12" r="8" /><path d="M12 7v5l3 2" /></>,
  dots: <><circle cx="5" cy="12" r="1" fill="currentColor" /><circle cx="12" cy="12" r="1" fill="currentColor" /><circle cx="19" cy="12" r="1" fill="currentColor" /></>,
  egg: <path d="M12 3c3.5 4.1 6 7 6 10a6 6 0 0 1-12 0c0-3 2.5-5.9 6-10Z" />,
  globe: <><circle cx="12" cy="12" r="8" /><path d="M4 12h16M12 4a12 12 0 0 1 0 16M12 4a12 12 0 0 0 0 16" /></>,
  leaf: <path d="M20 4C11 4 5 8 5 15c0 2.5 1.6 4 4 4 7 0 11-6 11-15ZM5 20c3-4 6-6 11-8" />,
  lightbulb: <><path d="M9 18h6M10 21h4M8 14a6 6 0 1 1 8 0c-1 1-1 2-1 3H9c0-1 0-2-1-3Z" /></>,
  message: <path d="M20 11.5a7.5 7.5 0 0 1-8 7.5 8.5 8.5 0 0 1-3.5-.8L4 20l1.5-4A7.3 7.3 0 0 1 4 11.5 7.5 7.5 0 0 1 12 4a7.5 7.5 0 0 1 8 7.5Z" />,
  plus: <path d="M12 5v14M5 12h14" />,
  pot: <><path d="M5 10h14v6a4 4 0 0 1-4 4H9a4 4 0 0 1-4-4z" /><path d="M7 7h10M9 4h6M3 12h2m14 0h2M15 7l3-4" /></>,
  search: <><circle cx="10.5" cy="10.5" r="5.5" /><path d="m15 15 4 4" /></>,
  settings: <><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.3 2.3-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-3v-.2a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1-2.3-2.3.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.6-1H4.8v-3H5a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1 2.3-2.3.1.1a1.7 1.7 0 0 0 1.9.3 1.7 1.7 0 0 0 1-1.6v-.2h3v.2a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1 2.3 2.3-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v3h-.2a1.7 1.7 0 0 0-1.6 1Z" /></>,
  sliders: <><path d="M4 7h16M4 17h16M8 4v6m8 4v6" /><circle cx="8" cy="7" r="2" /><circle cx="16" cy="17" r="2" /></>,
  sparkles: <><path d="m12 3 1.2 4.8L18 9l-4.8 1.2L12 15l-1.2-4.8L6 9l4.8-1.2zM19 15l.5 2 2 .5-2 .5-.5 2-.5-2-2-.5 2-.5z" /></>,
  trash: <><path d="M5 7h14M9 7V4h6v3m-8 0 1 13h8l1-13" /></>,
  video: <><rect x="3" y="6" width="13" height="12" rx="1" /><path d="m16 10 5-3v10l-5-3z" /></>,
  x: <path d="m6 6 12 12M18 6 6 18" />
};

export function Icon({ name, size = 20, strokeWidth = 1.7, className = '' }) {
  return (
    <svg className={`icon ${className}`} width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {paths[name] ?? paths.lightbulb}
    </svg>
  );
}
