import {
  useCallback,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";

const selections = new Map<string, unknown>();
export function useViewState<T>(
  key: string,
  initial: T,
): [T, Dispatch<SetStateAction<T>>] {
  const [value, setValue] = useState<T>(() =>
    selections.has(key) ? (selections.get(key) as T) : initial,
  );
  const update: Dispatch<SetStateAction<T>> = useCallback(
    (next) =>
      setValue((current) => {
        const result =
          typeof next === "function"
            ? (next as (value: T) => T)(current)
            : next;
        selections.set(key, result);
        return result;
      }),
    [key],
  );
  return [value, update];
}
export function clearViewState() {
  selections.clear();
}
