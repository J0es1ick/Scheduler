import styles from "./HomePage.module.css";
import { useEffect } from "react";
import { installSurfaceLight } from "../../../../shared/surfaceLight";
import { useReveal } from "../../shared/ui/useReveal";
import { usePublicInfo } from "../../features/public-info/usePublicInfo";
import { projectLinks } from "../../shared/config/project";
import { Footer } from "../../widgets/footer/Footer";
import { Header } from "../../widgets/header/Header";
import { ConnectorSection } from "./sections/connector/ConnectorSection";
import { HeroSection } from "./sections/hero/HeroSection";
import { StatisticsSection } from "./sections/statistics/StatisticsSection";
import { StatusSection } from "./sections/status/StatusSection";
import { TechnologiesSection } from "./sections/technologies/TechnologiesSection";
import { UniversitiesSection } from "./sections/universities/UniversitiesSection";
import { WorkflowSection } from "./sections/workflow/WorkflowSection";
import { PersonalFeatureSection } from "./sections/personal-feature/PersonalFeatureSection";
import { CallToActionSection } from "./sections/call-to-action/CallToActionSection";

export function HomePage() {
  const reveal = useReveal();
  useEffect(() => {
    if (reveal.current) return installSurfaceLight(reveal.current);
  }, [reveal]);
  const { info, loading } = usePublicInfo();
  const botURL = info?.bot_url || projectLinks.bot;
  const projectURL = info?.project_url || projectLinks.source;

  return (
    <div className={styles["public-site"]} ref={reveal}>
      <Header botURL={botURL} />
      <main>
        <HeroSection botURL={botURL} projectURL={projectURL} />
        <WorkflowSection />
        <PersonalFeatureSection botURL={botURL} />
        <UniversitiesSection
          botURL={botURL}
          universities={info?.university_names ?? []}
        />
        <StatusSection sources={info?.sources ?? []} />
        <StatisticsSection info={info} loading={loading} />
        <ConnectorSection projectURL={projectURL} />
        <TechnologiesSection />
        <CallToActionSection botURL={botURL} users={info?.users} />
      </main>
      <Footer botURL={botURL} projectURL={projectURL} />
    </div>
  );
}
