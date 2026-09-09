export function LogTabs({
  id,
  label,
  options,
  value,
  onChange,
}: {
  id: string;
  label: string;
  options: Array<{ value: string; label: string }>;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="log-tabs" role="tablist" aria-label={label}>
      {options.map((option, index) => (
        <button
          type="button"
          role="tab"
          id={`${id}-tab-${option.value}`}
          aria-controls={`${id}-panel-${option.value}`}
          aria-selected={value === option.value}
          tabIndex={value === option.value ? 0 : -1}
          key={option.value}
          onClick={() => onChange(option.value)}
          onKeyDown={(event) => {
            const next =
              event.key === "ArrowRight"
                ? (index + 1) % options.length
                : event.key === "ArrowLeft"
                  ? (index + options.length - 1) % options.length
                  : event.key === "Home"
                    ? 0
                    : event.key === "End"
                      ? options.length - 1
                      : -1;
            if (next < 0) return;
            event.preventDefault();
            onChange(options[next].value);
            document
              .getElementById(`${id}-tab-${options[next].value}`)
              ?.focus();
          }}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}
