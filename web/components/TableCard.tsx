// Wraps a data table in a card that scrolls sideways on narrow screens, so
// the page itself never does. Table cell styling lives in globals.css.
export function TableCard({ children }: { children: React.ReactNode }) {
  return <div className="table-card">{children}</div>;
}
