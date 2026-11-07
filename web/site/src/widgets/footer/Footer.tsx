import styles from "./Footer.module.css";
import sectionStyles from "../../shared/ui/section/Section.module.css";
import { ProjectMark } from "../../shared/ui/project-mark/ProjectMark";

interface FooterProps {
  botURL: string;
  projectURL: string;
}

export function Footer({ botURL, projectURL }: FooterProps) {
  return (
    <footer className={styles["public-footer"]}>
      <div className={sectionStyles["public-container"]}>
        <ProjectMark />
        <p>Открытый сервис актуального расписания для учебных заведений.</p>
        <div>
          <a href="#connectors">Connector SDK</a>
          <a href={projectURL} target="_blank" rel="noreferrer">
            GitHub
          </a>
          <a href={botURL} target="_blank" rel="noreferrer">
            Telegram
          </a>
        </div>
      </div>
    </footer>
  );
}
