"use client";

import { useEditor, EditorContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Placeholder from "@tiptap/extension-placeholder";
import {
    Bold,
    Italic,
    List,
    ListOrdered,
    Quote,
    Undo,
    Redo,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface RichTextEditorProps {
    value?: string;
    onChange?: (html: string) => void;
    placeholder?: string;
    className?: string;
}

const ToolbarButton = ({
    onClick,
    active,
    children,
    title,
}: {
    onClick: () => void;
    active?: boolean;
    children: React.ReactNode;
    title: string;
}) => (
    <button
        type="button"
        title={title}
        onMouseDown={(e) => {
            e.preventDefault();
            onClick();
        }}
        className={cn(
            "rounded-lg p-1.5 transition hover:bg-[var(--muted)]",
            active && "bg-[var(--muted)] text-[var(--primary)]",
        )}
    >
        {children}
    </button>
);

export function RichTextEditor({
    value,
    onChange,
    placeholder = "Describe your product...",
    className,
}: RichTextEditorProps) {
    const editor = useEditor({
        immediatelyRender: false,
        extensions: [
            StarterKit,
            Placeholder.configure({ placeholder }),
        ],
        content: value ?? "",
        editorProps: {
            attributes: {
                class:
                    "min-h-[160px] px-4 py-3 text-sm text-[var(--foreground)] focus:outline-none prose prose-sm max-w-none prose-p:my-1 prose-ul:my-1 prose-ol:my-1",
            },
        },
        onUpdate: ({ editor }) => {
            onChange?.(editor.getHTML());
        },
    });

    return (
        <div
            className={cn(
                "overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--panel)] focus-within:border-[var(--ring)] focus-within:ring-2 focus-within:ring-[var(--ring)]/30",
                className,
            )}
        >
            {/* Toolbar */}
            <div className="flex items-center gap-0.5 border-b border-[var(--border)] px-2 py-1.5">
                <ToolbarButton
                    title="Bold"
                    active={editor?.isActive("bold")}
                    onClick={() => editor?.chain().focus().toggleBold().run()}
                >
                    <Bold className="size-4" />
                </ToolbarButton>
                <ToolbarButton
                    title="Italic"
                    active={editor?.isActive("italic")}
                    onClick={() => editor?.chain().focus().toggleItalic().run()}
                >
                    <Italic className="size-4" />
                </ToolbarButton>
                <div className="mx-1 h-5 w-px bg-[var(--border)]" />
                <ToolbarButton
                    title="Bullet list"
                    active={editor?.isActive("bulletList")}
                    onClick={() => editor?.chain().focus().toggleBulletList().run()}
                >
                    <List className="size-4" />
                </ToolbarButton>
                <ToolbarButton
                    title="Numbered list"
                    active={editor?.isActive("orderedList")}
                    onClick={() => editor?.chain().focus().toggleOrderedList().run()}
                >
                    <ListOrdered className="size-4" />
                </ToolbarButton>
                <ToolbarButton
                    title="Blockquote"
                    active={editor?.isActive("blockquote")}
                    onClick={() => editor?.chain().focus().toggleBlockquote().run()}
                >
                    <Quote className="size-4" />
                </ToolbarButton>
                <div className="mx-1 h-5 w-px bg-[var(--border)]" />
                <ToolbarButton
                    title="Undo"
                    onClick={() => editor?.chain().focus().undo().run()}
                >
                    <Undo className="size-4" />
                </ToolbarButton>
                <ToolbarButton
                    title="Redo"
                    onClick={() => editor?.chain().focus().redo().run()}
                >
                    <Redo className="size-4" />
                </ToolbarButton>
            </div>

            <EditorContent editor={editor} />
        </div>
    );
}
