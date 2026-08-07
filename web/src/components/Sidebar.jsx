import { Icon } from './Icon.jsx';

const navItems = [
  ['ideas', 'Ideas', 'lightbulb'],
  ['recipes', 'Recipes', 'book'],
  ['production', 'Production', 'clapper']
];

export function Sidebar({ activeView, onNavigate, onCapture }) {
  return (
    <aside className="sidebar">
      <button className="brand" type="button" onClick={() => onNavigate('ideas')} aria-label="Sous Chef home">
        <span className="brand-mark"><Icon name="pot" size={38} strokeWidth={1.45} /></span>
        <span>Sous Chef</span>
      </button>

      <nav className="nav-list" aria-label="Primary navigation">
        {navItems.map(([id, label, icon]) => (
          <button className={`nav-item ${activeView === id ? 'is-active' : ''}`} type="button" onClick={() => onNavigate(id)} key={id}>
            <Icon name={icon} size={21} />
            <span>{label}</span>
          </button>
        ))}
      </nav>

      <button className="button button-primary sidebar-new" type="button" onClick={onCapture}>
        <Icon name="plus" size={19} />
        New idea
        <kbd>⌘ N</kbd>
      </button>

      <div className="sidebar-footer">
        <button className="nav-item" type="button"><Icon name="settings" size={20} />Settings</button>
        <button className="nav-item" type="button"><span className="sun-icon">☼</span>Light mode<Icon name="chevron" size={16} /></button>
      </div>
    </aside>
  );
}
