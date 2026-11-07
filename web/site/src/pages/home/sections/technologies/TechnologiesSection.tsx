import { classNames } from "../../../../../../shared/classNames";
import sectionStyles from "../../../../shared/ui/section/Section.module.css";
import styles from "./TechnologiesSection.module.css";
import { technologyGroups } from "../../content";

export function TechnologiesSection() {
  return (
    <section className={classNames(sectionStyles["public-section"], styles["public-technologies"])} id="technologies">
      <div className={sectionStyles["public-container"]} data-reveal>
        <details>
          <summary>На чём работает Scheduler</summary>
          <div className={styles["public-tech-grid"]}>
            {technologyGroups.map((group) => {
              const Icon = group.icon;
              return (
                <article key={group.title} className={styles["public-tech-card"]}>
                  <div>
                    <Icon size={21} />
                    <h3>{group.title}</h3>
                  </div>
                  <ul>
                    {group.items.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                </article>
              );
            })}
          </div>
        </details>
      </div>
    </section>
  );
}
