import { useEffect, useRef, useState } from "react";
import { ArrowUpRight, Menu, X } from "lucide-react";
import { ProjectMark } from "../shared/ui/ProjectMark";

const navigation = [
  { id: "about", label: "О проекте" },
  { id: "how-it-works", label: "Как работает" },
  { id: "universities", label: "Вузы" },
  { id: "connectors", label: "Разработчикам" },
  { id: "status", label: "Статус" },
];

interface HeaderProps {
  botURL: string;
}

export function Header({ botURL }: HeaderProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const toggle = useRef<HTMLButtonElement>(null);
  const [activeSection, setActiveSection] = useState("about");
  useEffect(() => {
    if (!("IntersectionObserver" in window)) return;
    const visible = new Map<string, Element>();
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) visible.set(entry.target.id, entry.target);
        else visible.delete(entry.target.id);
      });
      const closest = [...visible.values()].sort((a, b) =>
        Math.abs(a.getBoundingClientRect().top - 80) - Math.abs(b.getBoundingClientRect().top - 80),
      )[0];
      if (closest) setActiveSection(closest.id);
    }, { rootMargin: "-76px 0px -55% 0px", threshold: 0 });
    navigation.forEach(({ id }) => {
      const section = document.getElementById(id);
      if (section) observer.observe(section);
    });
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    const desktop = window.matchMedia("(min-width: 761px)");
    const closeOnDesktop = () => {
      if (desktop.matches) setMenuOpen(false);
    };
    desktop.addEventListener("change", closeOnDesktop);
    return () => desktop.removeEventListener("change", closeOnDesktop);
  }, []);
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
          <span className="public-menu-icon" data-open={menuOpen} aria-hidden="true">
            <Menu size={21} />
            <X size={21} />
          </span>
        </button>
        <div className="public-nav-shell" data-open={menuOpen}>
          <nav
            id="public-navigation"
            className={menuOpen ? "is-open" : ""}
            aria-label="Навигация"
          >
            {navigation.map(({ id, label }) => (
              <a
                key={id}
                href={`#${id}`}
                aria-current={activeSection === id ? "location" : undefined}
                onClick={closeMenu}
              >
                {label}
              </a>
            ))}
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
      </div>
    </header>
  );
}
