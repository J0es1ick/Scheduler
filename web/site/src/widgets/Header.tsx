import { useEffect, useRef, useState } from "react";
import { ArrowUpRight, Menu, X } from "lucide-react";
import { ProjectMark } from "../shared/ui/ProjectMark";

interface HeaderProps {
  botURL: string;
}

export function Header({ botURL }: HeaderProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const toggle = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && menuOpen) {
        setMenuOpen(false);
        toggle.current?.focus();
      }
    };
    document.addEventListener("keydown", escape);
    return () => document.removeEventListener("keydown", escape);
  }, [menuOpen]);
  const closeMenu = () => setMenuOpen(false);

  return (
    <header className="public-header">
      <div className="public-container public-header-inner">
        <ProjectMark />
        <button
          ref={toggle}
          aria-controls="public-navigation"
          className="public-menu-button"
          type="button"
          onClick={() => setMenuOpen((current) => !current)}
          aria-expanded={menuOpen}
          aria-label={menuOpen ? "Закрыть меню" : "Открыть меню"}
        >
          {menuOpen ? <X size={21} /> : <Menu size={21} />}
        </button>
        <nav
          id="public-navigation"
          className={menuOpen ? "is-open" : ""}
          aria-label="Навигация"
        >
          <a href="#about" onClick={closeMenu}>
            О проекте
          </a>
          <a href="#how-it-works" onClick={closeMenu}>
            Как работает
          </a>
          <a href="#technologies" onClick={closeMenu}>
            Технологии
          </a>
          <a href="#connectors" onClick={closeMenu}>
            Разработчикам
          </a>
          <a href="#status" onClick={closeMenu}>
            Статус
          </a>
          <a
            className="public-header-cta"
            href={botURL}
            target="_blank"
            rel="noreferrer"
          >
            Открыть бота
            <ArrowUpRight size={16} />
          </a>
        </nav>
      </div>
    </header>
  );
}
