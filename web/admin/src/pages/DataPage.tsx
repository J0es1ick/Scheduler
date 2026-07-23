import { useEffect, useState } from "react";
import {
  BookOpen,
  CalendarDays,
  ChevronRight,
  Layers3,
  UsersRound,
} from "lucide-react";
import { api } from "../api";
import {
  EmptyBlock,
  ErrorBlock,
  formatDate,
  LoadingBlock,
  number,
  PaginationControls,
  SearchField,
  SectionTitle,
  SourceGlyph,
} from "../components";
import { useDebounced, useRemote } from "../hooks";
import type { GroupView } from "../types";

const lessonLabels: Record<string, string> = {
  lecture: "Лекция",
  practice: "Практика",
  lab: "Лабораторная",
  seminar: "Семинар",
  exam: "Экзамен",
  credit: "Зачёт",
  consultation: "Консультация",
};

export function DataPage() {
  const [tab, setTab] = useState<"groups" | "lessons">("groups");
  const [query, setQuery] = useState("");
  const [university, setUniversity] = useState("");
  const [page, setPage] = useState(1);
  const [selectedGroup, setSelectedGroup] = useState<GroupView | null>(null);
  const debounced = useDebounced(query);
  const universities = useRemote(() => api.universities(), []);
  const groups = useRemote(
    () => api.groups({ page, q: debounced, university }),
    [page, debounced, university],
    { enabled: tab === "groups" },
  );
  const lessons = useRemote(
    () =>
      api.lessons({ page, q: debounced, university, group: selectedGroup?.id }),
    [page, debounced, university, selectedGroup?.id],
    { enabled: tab === "lessons" },
  );

  useEffect(() => setPage(1), [debounced, university, tab, selectedGroup?.id]);

  const openGroup = (group: GroupView) => {
    setSelectedGroup(group);
    setQuery("");
    setTab("lessons");
  };

  const switchTab = (nextTab: "groups" | "lessons") => {
    setTab(nextTab);
    setQuery("");
    if (nextTab === "groups") setSelectedGroup(null);
  };

  return (
    <div className="page-stack data-page">
      <div className="data-toolbar">
        <div className="segmented-control">
          <button
            className={tab === "groups" ? "is-active" : ""}
            onClick={() => switchTab("groups")}
          >
            <UsersRound size={17} /> Группы
          </button>
          <button
            className={tab === "lessons" ? "is-active" : ""}
            onClick={() => switchTab("lessons")}
          >
            <BookOpen size={17} /> Занятия
          </button>
        </div>
        <SearchField
          value={query}
          onChange={setQuery}
          placeholder={
            tab === "groups"
              ? "Номер группы или ID"
              : "Предмет, преподаватель, аудитория"
          }
        />
        <select
          className="select-control"
          value={university}
          onChange={(event) => setUniversity(event.target.value)}
        >
          <option value="">Все университеты</option>
          {(universities.data ?? []).map((item) => (
            <option value={item.id} key={item.id}>
              {item.name}
            </option>
          ))}
        </select>
      </div>

      {selectedGroup && tab === "lessons" && (
        <div className="active-filter">
          <SourceGlyph name={selectedGroup.university_name} small />
          <span>
            Расписание группы <strong>{selectedGroup.name}</strong>
          </span>
          <button onClick={() => setSelectedGroup(null)}>Показать все</button>
        </div>
      )}

      {tab === "groups" ? (
        <section className="card-surface table-card">
          <SectionTitle eyebrow="Справочник" title="Учебные группы" />
          {groups.loading && !groups.data ? (
            <LoadingBlock rows={6} />
          ) : groups.error ? (
            <ErrorBlock message={groups.error} retry={groups.reload} />
          ) : !groups.data?.items.length ? (
            <EmptyBlock
              title="Группы не найдены"
              text="Попробуйте изменить поисковый запрос."
            />
          ) : (
            <>
              <div className="responsive-table groups-table">
                <div className="table-head">
                  <span>Группа</span>
                  <span>Университет</span>
                  <span>Занятия</span>
                  <span>Обновлено</span>
                  <span />
                </div>
                {groups.data.items.map((group) => (
                  <button
                    className="table-row"
                    key={group.id}
                    onClick={() => openGroup(group)}
                  >
                    <div data-label="Группа">
                      <SourceGlyph name={group.university_name} small />
                      <div>
                        <strong>{group.name}</strong>
                        <span>{group.id}</span>
                      </div>
                    </div>
                    <div data-label="Университет">
                      <strong>{group.university_name}</strong>
                    </div>
                    <div data-label="Занятия">
                      <strong>{number.format(group.lesson_count)}</strong>
                      <span>в снимке</span>
                    </div>
                    <div data-label="Обновлено">
                      <strong>{formatDate(group.updated_at)}</strong>
                    </div>
                    <div>
                      <ChevronRight size={18} />
                    </div>
                  </button>
                ))}
              </div>
              <PaginationControls
                pagination={groups.data.pagination}
                onPage={setPage}
              />
            </>
          )}
        </section>
      ) : (
        <section className="lesson-browser">
          <SectionTitle
            eyebrow="Снимок"
            title={
              selectedGroup ? `Группа ${selectedGroup.name}` : "Все занятия"
            }
          />
          {lessons.loading && !lessons.data ? (
            <LoadingBlock rows={6} />
          ) : lessons.error ? (
            <ErrorBlock message={lessons.error} retry={lessons.reload} />
          ) : !lessons.data?.items.length ? (
            <EmptyBlock
              title="Занятия не найдены"
              text="В актуальном снимке нет подходящих записей."
            />
          ) : (
            <>
              <div className="lesson-grid">
                {lessons.data.items.map((lesson) => (
                  <article className="lesson-card" key={lesson.id}>
                    <div className="lesson-time">
                      <strong>{lesson.time_start}</strong>
                      <span>{lesson.time_end}</span>
                      <i />
                    </div>
                    <div className="lesson-main">
                      <span className="lesson-type">
                        {lessonLabels[lesson.type] ?? lesson.type}
                      </span>
                      <h3>{lesson.subject}</h3>
                      <p>
                        {lesson.teacher || "Преподаватель не указан"} ·{" "}
                        {lesson.room || "аудитория не указана"}
                      </p>
                      <div>
                        <span>
                          <Layers3 size={14} /> {lesson.group_name}
                        </span>
                        <span>
                          <CalendarDays size={14} />{" "}
                          {lesson.special_date
                            ? formatDate(lesson.special_date)
                            : `${formatDate(lesson.valid_from)} — ${formatDate(lesson.valid_to)}`}
                        </span>
                      </div>
                    </div>
                  </article>
                ))}
              </div>
              <PaginationControls
                pagination={lessons.data.pagination}
                onPage={setPage}
              />
            </>
          )}
        </section>
      )}
    </div>
  );
}
