import { classNames } from "../../../../shared/classNames";
import styles from "./PersonalApp.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import personalUIStyles from "../../features/personal-shared/PersonalUI.module.css";
import { PersonalCalendar } from "../../features/personal-schedule/PersonalCalendar";
import { useEffect, useState } from "react";
import { CalendarDays, Bell, NotebookPen, ArrowUpRight, Shield } from "lucide-react";
import { useTheme } from "../../app/useTheme";
import { useMiniAppViewport } from "../../app/useMiniApp";
import { ThemeSwitch } from "../../ui/theme-switch/ThemeSwitch";
import { LoadingBlock } from "../../ui";
import { login, type Profile } from "../../features/personal-shared/api";

import { NotificationCenter } from "../../features/personal-notifications/NotificationCenter";
import { Notes } from "../../features/personal-notes/Notes";

export default function PersonalApp() {
  const theme = useTheme();
  useMiniAppViewport();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [chosen, setChosen] = useState(() => new URLSearchParams(window.location.search).has("section"));
  const [section, setSection] = useState(() => new URLSearchParams(window.location.search).get("section") || "schedule");
  useEffect(() => {
    const back = window.Telegram?.WebApp?.BackButton;
    const update = () => {
      if ((profile?.is_admin && chosen) || document.querySelector(`.${personalUIStyles["personal-dialog"]}[open]`)) back?.show();
      else back?.hide();
    };
    const onBack = () => {
      const dialog = document.querySelector<HTMLDialogElement>(`.${personalUIStyles["personal-dialog"]}[open]`);
      if (dialog) dialog.dispatchEvent(new Event("cancel", { cancelable: true }));
      else setChosen(false);
    };
    update();
    back?.onClick(onBack);
    window.addEventListener("scheduler:dialog-state", update);
    return () => {
      back?.offClick(onBack);
      window.removeEventListener("scheduler:dialog-state", update);
    };
  }, [chosen, profile?.is_admin]);
  useEffect(() => {
    let active = true;
    window.Telegram?.WebApp?.ready?.();
    window.Telegram?.WebApp?.expand?.();
    void login()
      .then((result) => {
        if (active) {
          setProfile(result);
          setError("");
        }
      })
      .catch((caught) => {
        if (active)
          setError(
            caught instanceof Error ? caught.message : "Не удалось войти",
          );
      });
    return () => {
      active = false;
    };
  }, [attempt]);
  return (
    <main className={styles["personal-app"]}>
      <header className={styles["personal-header"]}>
        <a href="/app" className={styles["personal-brand"]}>
          <span className={styles["personal-brand-mark"]}><CalendarDays size={22} aria-hidden="true" /></span>
          <span><strong>Моё расписание</strong><small>Scheduler · только для вас</small></span>
        </a>
        <ThemeSwitch {...theme} />
      </header>
      {!profile ? (
        <section className={styles["personal-welcome"]}>
          <h1>Расписание под ваш день</h1>
          <p>{error || "Открываем ваш профиль…"}</p>
          {!error && <LoadingBlock rows={2} />}
          {error && (
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
              onClick={() => setAttempt((value) => value + 1)}
            >
              Повторить вход
            </button>
          )}
        </section>
      ) : profile.is_admin && !chosen ? (
        <section className={styles["personal-welcome"]}>
          <p className={personalUIStyles["personal-eyebrow"]}>{profile.name || "Здравствуйте"}</p>
          <h1>Куда перейдём?</h1>
          <div className={styles["personal-destinations"]}>
            <button onClick={() => setChosen(true)}>
              <CalendarDays aria-hidden="true" />
              <strong>Личное расписание</strong>
              <span>Настроить занятия только для себя</span>
              <ArrowUpRight className={styles["personal-destination-arrow"]} size={20} aria-hidden="true" />
            </button>
            <a href="/#/editor">
              <Shield aria-hidden="true" />
              <strong>Админка</strong>
              <span>Управлять общим расписанием и сервисом</span>
              <ArrowUpRight className={styles["personal-destination-arrow"]} size={20} aria-hidden="true" />
            </a>
          </div>
        </section>
      ) : (
        <div className={styles["personal-workspace"]}>
          <div className={styles["personal-intro"]}>
            {profile.is_admin && (
              <button
                className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                onClick={() => setChosen(false)}
              >
                ← К выбору сервиса
              </button>
            )}
            <h1>{section === "notifications" ? "Уведомления" : section === "notes" ? "Ваши заметки" : "Ваш день"}</h1>
            <p>Расписание, личные правки и задания — в одном месте.</p>
          </div>
          <nav className={styles["personal-tabs"]} aria-label="Личное расписание">{([{ id: "schedule", label: "Расписание", Icon: CalendarDays }, { id: "notes", label: "Заметки", Icon: NotebookPen }, { id: "notifications", label: "Уведомления", Icon: Bell }]).map(({ id, label, Icon }) => <button key={id} aria-current={section === id ? "page" : undefined} onClick={() => setSection(id)}><Icon size={18} /><span>{label}</span></button>)}</nav>
          {section === "notifications" ? <NotificationCenter /> : section === "notes" ? <Notes targets={profile.targets} /> : <PersonalCalendar profile={profile} />}
        </div>
      )}
    </main>
  );
}
