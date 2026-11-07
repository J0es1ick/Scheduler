import styles from "./PaginationControls.module.css";
import { number } from "../../shared/lib/format";

import { ArrowLeft, ArrowRight } from "lucide-react";
import type { Pagination } from "../../shared/model";

export function PaginationControls({
  pagination,
  onPage,
}: {
  pagination: Pagination;
  onPage: (page: number) => void;
}) {
  const pages = Math.max(1, Math.ceil(pagination.total / pagination.page_size));
  return (
    <div className={styles["pagination"]}>
      <span>{number.format(pagination.total)} записей</span>
      <div>
        <button
          disabled={pagination.page <= 1}
          onClick={() => onPage(pagination.page - 1)}
          aria-label="Назад"
        >
          <ArrowLeft size={17} />
        </button>
        <strong>{pagination.page}</strong>
        <span>/ {pages}</span>
        <button
          disabled={pagination.page >= pages}
          onClick={() => onPage(pagination.page + 1)}
          aria-label="Вперёд"
        >
          <ArrowRight size={17} />
        </button>
      </div>
    </div>
  );
}
