import type { ReactNode } from "react";

/** PageHeader is the top of a page: its title, under the window's title bar, and its actions. */
export function PageHeader({ title, children, sub }: { title: string; children?: ReactNode; sub?: ReactNode }) {
  return (
    <header className="page-header">
      <h1>{title}</h1>
      {sub}
      <div className="actions">{children}</div>
    </header>
  );
}
