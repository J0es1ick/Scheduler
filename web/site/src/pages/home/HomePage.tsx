import { usePublicInfo } from "../../features/public-info/usePublicInfo";
import { projectLinks } from "../../shared/config/project";
import { Footer } from "../../widgets/Footer";
import { Header } from "../../widgets/Header";
import { CallToActionSection } from "./sections/CallToActionSection";
import { HeroSection } from "./sections/HeroSection";
import { StatisticsSection } from "./sections/StatisticsSection";
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
        <StatisticsSection info={info} loading={loading} />
        <WorkflowSection />
        <UniversitiesSection
          botURL={botURL}
          universities={info?.university_names ?? []}
        />
        <TechnologiesSection />
        <CallToActionSection botURL={botURL} users={info?.users} />
      </main>
      <Footer botURL={botURL} projectURL={projectURL} />
    </div>
  );
}
