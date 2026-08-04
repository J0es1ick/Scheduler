export type ScheduleWeekFilter = "all" | "odd" | "even" | "date";

export type WeekTypedLesson = {
  week_type: "every" | "odd" | "even" | "date";
};

export type ScheduleWeekSection<T extends WeekTypedLesson> = {
  key: ScheduleWeekFilter;
  title: string;
  note: string;
  lessons: T[];
};

export function hasAlternatingWeeks<T extends WeekTypedLesson>(lessons: T[]) {
  return lessons.some(
    (lesson) => lesson.week_type === "odd" || lesson.week_type === "even",
  );
}

export function buildScheduleWeekSections<T extends WeekTypedLesson>(
  lessons: T[],
  filter: ScheduleWeekFilter = "all",
  forceAlternating = false,
): ScheduleWeekSection<T>[] {
  if (filter === "odd" || filter === "even") {
    return [alternatingSection(lessons, filter)];
  }
  if (filter === "date") {
    return [datedSection(lessons)];
  }
  if (!forceAlternating && !hasAlternatingWeeks(lessons)) {
    return [{ key: "all", title: "", note: "", lessons }];
  }

  const sections: ScheduleWeekSection<T>[] = [
    alternatingSection(lessons, "odd"),
    alternatingSection(lessons, "even"),
  ];
  const dated = lessons.filter((lesson) => lesson.week_type === "date");
  if (dated.length) sections.push(datedSection(lessons));
  return sections;
}

function alternatingSection<T extends WeekTypedLesson>(
  lessons: T[],
  week: "odd" | "even",
): ScheduleWeekSection<T> {
  return {
    key: week,
    title: week === "odd" ? "Нечётная неделя" : "Чётная неделя",
    note: "Общие занятия включены в обе недели.",
    lessons: lessons.filter(
      (lesson) => lesson.week_type === "every" || lesson.week_type === week,
    ),
  };
}

function datedSection<T extends WeekTypedLesson>(
  lessons: T[],
): ScheduleWeekSection<T> {
  return {
    key: "date",
    title: "Занятия по датам",
    note: "Разовые занятия и события показаны отдельно.",
    lessons: lessons.filter((lesson) => lesson.week_type === "date"),
  };
}
