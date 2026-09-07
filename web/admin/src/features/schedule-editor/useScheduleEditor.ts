import { useViewState } from "../../hooks/useViewState";
import { useEffect, useMemo, useState } from "react";
import { APIError, api } from "../../api";
import type { ToastMessage } from "../../components";
import { useDebounced, useRemote } from "../../hooks";
import type {
  EditorLesson,
  EditorSchedule,
  GroupView,
  LessonMutationPayload,
} from "../../types";
import { buildScheduleWeekSections } from "../schedule-shared/weekSections";
import type { LessonForm, WeekFilter } from "./model";

export function useScheduleEditor(
  notify: (text: string, tone?: ToastMessage["tone"]) => void,
) {
  const [university, setUniversity] = useViewState("editor:university", "");
  const [groupQuery, setGroupQuery] = useViewState("editor:groupQuery", "");
  const [groupSearchOpen, setGroupSearchOpen] = useState(false);
  const [selectedGroupID, setSelectedGroupID] = useViewState(
    "editor:selectedGroupID",
    "",
  );
  const [week, setWeek] = useViewState<WeekFilter>("editor:week", "all");
  const [lessonQuery, setLessonQuery] = useViewState("editor:lessonQuery", "");
  const [dialog, setDialog] = useState<{
    lesson: EditorLesson | null;
    day: number;
  } | null>(null);
  const [conflict, setConflict] = useState<EditorLesson | null | undefined>(
    undefined,
  );
  useEffect(() => setConflict(undefined), [dialog]);
  const [deleteTarget, setDeleteTarget] = useState<EditorLesson | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<EditorLesson | null>(null);
  const [changesOpen, setChangesOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const debouncedGroupQuery = useDebounced(groupQuery);

  const universities = useRemote(() => api.universities(), []);
  const groups = useRemote(
    () =>
      api.groups({
        page: 1,
        pageSize: 20,
        q: debouncedGroupQuery,
        university,
        status: "active",
        selector: true,
      }),
    [debouncedGroupQuery, university],
    { enabled: debouncedGroupQuery.trim().length > 0 },
  );
  const schedule = useRemote<EditorSchedule | null>(
    () =>
      selectedGroupID
        ? api.editorSchedule(selectedGroupID)
        : Promise.resolve(null),
    [selectedGroupID],
  );

  const searchedLessons = useMemo(() => {
    const query = lessonQuery.trim().toLocaleLowerCase("ru-RU");
    return (schedule.data?.lessons ?? []).filter((lesson) => {
      const haystack =
        `${lesson.subject} ${lesson.teacher} ${lesson.room}`.toLocaleLowerCase(
          "ru-RU",
        );
      return !query || haystack.includes(query);
    });
  }, [lessonQuery, schedule.data]);

  const weekSections = useMemo(
    () => buildScheduleWeekSections(searchedLessons, week),
    [searchedLessons, week],
  );

  const editorLessons = schedule.data?.lessons ?? [];
  const deletedLessons = schedule.data?.deleted_lessons ?? [];
  const manualLessons = editorLessons.filter(
    (lesson) => lesson.origin === "manual",
  );

  async function saveLesson(form: LessonForm) {
    if (!schedule.data || !dialog) return;
    setBusy(true);
    const payload: LessonMutationPayload = {
      group_id: schedule.data.group.id,
      ...form,
      expected_updated_at: dialog.lesson?.updated_at,
    };
    try {
      if (dialog.lesson) {
        await api.updateEditorLesson(dialog.lesson.id, payload);
        notify("Занятие обновлено");
      } else {
        await api.createEditorLesson(payload);
        notify("Занятие добавлено");
      }
      setDialog(null);
      await schedule.reload();
    } catch (caught) {
      if (
        caught instanceof APIError &&
        caught.status === 409 &&
        dialog.lesson
      ) {
        try {
          const latest = await api.editorSchedule(schedule.data.group.id);
          const originalID = dialog.lesson.base_lesson_id ?? dialog.lesson.id;
          setConflict(
            latest.lessons.find(
              (item) =>
                item.id === dialog.lesson?.id ||
                item.id === originalID ||
                item.base_lesson_id === originalID,
            ) ?? null,
          );
        } catch {
          notify(
            "Не удалось загрузить новую версию для сравнения. Ваш ввод сохранён.",
            "error",
          );
        }
      }
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось сохранить занятие",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }

  async function deleteLesson() {
    if (!deleteTarget) return;
    setBusy(true);
    try {
      await api.deleteEditorLesson(deleteTarget);
      notify(
        deleteTarget.base_lesson_id || deleteTarget.origin === "parsed"
          ? "Занятие скрыто. Исходную версию можно восстановить"
          : "Занятие удалено",
      );
      setDeleteTarget(null);
      await schedule.reload();
    } catch (caught) {
      notify(
        caught instanceof Error ? caught.message : "Не удалось удалить занятие",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }

  async function restoreLesson() {
    if (!restoreTarget) return;
    setBusy(true);
    try {
      await api.restoreEditorLesson(restoreTarget.id);
      notify("Версия с сайта восстановлена");
      setRestoreTarget(null);
      await schedule.reload();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось восстановить занятие",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }

  function changeUniversity(value: string) {
    setUniversity(value);
    setSelectedGroupID("");
    setGroupQuery("");
    setGroupSearchOpen(false);
  }

  function selectGroup(group: GroupView) {
    setSelectedGroupID(group.id);
    setGroupQuery(group.name);
    setGroupSearchOpen(false);
  }

  return {
    university,
    groupQuery,
    groupSearchOpen,
    selectedGroupID,
    week,
    lessonQuery,
    conflict,
    rebase: () => {
      if (conflict) setDialog({ lesson: conflict, day: dialog?.day ?? 1 });
    },
    dialog,
    deleteTarget,
    restoreTarget,
    changesOpen,
    exportOpen,
    busy,
    universities,
    groups,
    schedule,
    groupResults: groups.data?.items ?? [],
    weekSections,
    editorLessons,
    deletedLessons,
    manualLessons,
    setGroupQuery,
    setGroupSearchOpen,
    setWeek,
    setLessonQuery,
    setDialog,
    setDeleteTarget,
    setRestoreTarget,
    setChangesOpen,
    setExportOpen,
    changeUniversity,
    selectGroup,
    saveLesson,
    deleteLesson,
    restoreLesson,
  };
}
