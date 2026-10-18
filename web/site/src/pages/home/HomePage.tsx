import { useEffect } from "react";
import { installSurfaceLight } from "../../../../shared/surfaceLight";
import { useReveal } from "../../shared/ui/useReveal";
import { usePublicInfo } from "../../features/public-info/usePublicInfo";
import { projectLinks } from "../../shared/config/project";
import { Footer } from "../../widgets/Footer";
import { Header } from "../../widgets/Header";
import { ConnectorSection } from "./sections/ConnectorSection";
import { HeroSection } from "./sections/HeroSection";
import { StatisticsSection } from "./sections/StatisticsSection";
import { StatusSection } from "./sections/StatusSection";
import { TechnologiesSection } from "./sections/TechnologiesSection";
import { UniversitiesSection } from "./sections/UniversitiesSection";
import { WorkflowSection } from "./sections/WorkflowSection";
import { PersonalFeatureSection } from "./sections/PersonalFeatureSection";
import { CallToActionSection } from "./sections/CallToActionSection";

export function HomePage() {
  const reveal = useReveal();
  useEffect(() => {
    if (reveal.current) return installSurfaceLight(reveal.current);
  }, [reveal]);
  const { info, loading } = usePublicInfo();
  const botURL = info?.bot_url || projectLinks.bot;
  const projectURL = info?.project_url || projectLinks.source;

  return (
    <div className="public-site" ref={reveal}>
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
