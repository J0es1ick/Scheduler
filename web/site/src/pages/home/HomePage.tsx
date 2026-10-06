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

export function HomePage() {
  const { info, loading } = usePublicInfo();
  const botURL = info?.bot_url || projectLinks.bot;
  const projectURL = info?.project_url || projectLinks.source;

  return (
    <div className="public-site">
      <Header botURL={botURL} />
      <main>
        <HeroSection botURL={botURL} projectURL={projectURL} />
        <WorkflowSection />
        <UniversitiesSection
          botURL={botURL}
          universities={info?.university_names ?? []}
        />
        <StatusSection sources={info?.sources ?? []} />
        <StatisticsSection info={info} loading={loading} />
        <ConnectorSection projectURL={projectURL} />
        <TechnologiesSection />
      </main>
      <Footer botURL={botURL} projectURL={projectURL} />
    </div>
  );
}
