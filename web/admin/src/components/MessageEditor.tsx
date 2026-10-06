import { useEffect, useState } from "react";
import {
  EditorContent,
  useEditor,
  useEditorState,
  type JSONContent,
} from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import {
  Bold,
  Italic,
  Link,
  List,
  ListOrdered,
  Undo2,
  Redo2,
} from "lucide-react";

export function MessageEditor({
  initial,
  disabled,
  onChange,
}: {
  initial: JSONContent;
  disabled: boolean;
  onChange: (document: JSONContent) => void;
}) {
  const [link, setLink] = useState("");
  const [showLink, setShowLink] = useState(false);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: false,
        blockquote: false,
        code: false,
        codeBlock: false,
        horizontalRule: false,
        strike: false,
        underline: false,
        link: { openOnClick: false, protocols: ["https", "http", "mailto"] },
      }),
    ],
    content: initial,
    editable: !disabled,
    editorProps: {
      attributes: {
        role: "textbox",
        "aria-label": "Текст сообщения",
        "aria-multiline": "true",
      },
    },
    onUpdate: ({ editor }) => onChange(editor.getJSON()),
  });
  useEffect(() => {
    editor?.setEditable(!disabled, false);
  }, [editor, disabled]);
  const state = useEditorState({
    editor,
    selector: ({ editor }) =>
      editor
        ? {
            bold: editor.isActive("bold"),
            italic: editor.isActive("italic"),
            bullet: editor.isActive("bulletList"),
            ordered: editor.isActive("orderedList"),
            length: editor.getText().length,
          }
        : null,
  });
  if (!editor) return null;
  return (
    <div className={`message-editor ${disabled ? "is-disabled" : ""}`}>
      <div className="message-toolbar" aria-label="Оформление текста">
        <button
          type="button"
          disabled={disabled}
          title="Жирный"
          aria-label="Жирный"
          aria-pressed={state?.bold}
          onClick={() => editor.chain().focus().toggleBold().run()}
        >
          <Bold size={17} />
        </button>
        <button
          type="button"
          disabled={disabled}
          title="Курсив"
          aria-label="Курсив"
          aria-pressed={state?.italic}
          onClick={() => editor.chain().focus().toggleItalic().run()}
        >
          <Italic size={17} />
        </button>
        <button
          type="button"
          disabled={disabled}
          title="Ссылка"
          aria-label="Ссылка"
          onClick={() => setShowLink(!showLink)}
        >
          <Link size={17} />
        </button>
        <button
          type="button"
          disabled={disabled}
          title="Маркированный список"
          aria-label="Маркированный список"
          aria-pressed={state?.bullet}
          onClick={() => editor.chain().focus().toggleBulletList().run()}
        >
          <List size={17} />
        </button>
        <button
          type="button"
          disabled={disabled}
          title="Нумерованный список"
          aria-label="Нумерованный список"
          aria-pressed={state?.ordered}
          onClick={() => editor.chain().focus().toggleOrderedList().run()}
        >
          <ListOrdered size={17} />
        </button>
        <button
          type="button"
          disabled={disabled}
          title="Отменить ввод"
          aria-label="Отменить ввод"
          onClick={() => editor.chain().focus().undo().run()}
        >
          <Undo2 size={17} />
        </button>
        <button
          type="button"
          disabled={disabled}
          title="Повторить ввод"
          aria-label="Повторить ввод"
          onClick={() => editor.chain().focus().redo().run()}
        >
          <Redo2 size={17} />
        </button>
        <details className="emoji-picker">
          <summary aria-label="Выбрать эмодзи">☺</summary>
          <div>
            {[
              "🎉",
              "📅",
              "🔔",
              "✨",
              "✅",
              "🛠",
              "📚",
              "💬",
              "🚀",
              "👋",
              "❤️",
              "💡",
              "📎",
              "📢",
              "🎓",
              "👍",
              "📝",
              "⏰",
              "🔍",
              "🙌",
            ].map((emoji) => (
              <button
                key={emoji}
                type="button"
                disabled={disabled}
                onClick={() =>
                  editor.chain().focus().insertContent(emoji).run()
                }
              >
                {emoji}
              </button>
            ))}
          </div>
        </details>
      </div>
      {showLink && (
        <div className="message-link">
          <input
            aria-label="Адрес ссылки"
            disabled={disabled}
            type="url"
            value={link}
            onChange={(e) => setLink(e.target.value)}
            placeholder="https://…"
          />
          <button
            type="button"
            className="button button-ghost"
            disabled={disabled || !/^(https?:\/\/|mailto:)/i.test(link)}
            onClick={() => {
              editor
                .chain()
                .focus()
                .extendMarkRange("link")
                .setLink({ href: link })
                .run();
              setShowLink(false);
            }}
          >
            Применить
          </button>
          <button
            type="button"
            className="text-button"
            disabled={disabled}
            onClick={() => {
              editor.chain().focus().unsetLink().run();
              setShowLink(false);
            }}
          >
            Убрать ссылку
          </button>
        </div>
      )}
      <fieldset disabled={disabled} className="message-input">
        <EditorContent editor={editor} />
      </fieldset>
      <div className="message-editor-footer">
        <span>Можно вставить эмодзи с клавиатуры</span>
        <span className={(state?.length ?? 0) > 4096 ? "is-error" : ""}>
          {state?.length ?? 0} / 4096
        </span>
      </div>
    </div>
  );
}
