"use client";

import { useCallback, useEffect, useState, useRef } from "react";
import { useDropzone } from "react-dropzone";
import * as Dialog from "@radix-ui/react-dialog";
import { Search, Image as ImageIcon, CheckCircle2, Loader2, UploadCloud, FileVideo, Tags } from "lucide-react";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Input, Button } from "@/components/ui/primitives";
import { api, publicUploadUrl } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import type { MediaAsset } from "@/types/api";

interface MediaLibraryModalProps {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    mode?: "single" | "multiple";
    value?: MediaAsset[];
    onChange?: (items: MediaAsset[]) => void;
}

const EMPTY_MEDIA: MediaAsset[] = [];

function normalizeSelection(items: MediaAsset[] | undefined): MediaAsset[] {
    if (!Array.isArray(items)) return [];
    const out: MediaAsset[] = [];
    const seen = new Set<string>();
    for (const item of items) {
        if (!item) continue;
        const key = (typeof item.url === "string" && item.url) || (typeof item.id === "string" && item.id) || "";
        if (!key) continue;
        if (seen.has(key)) continue;
        seen.add(key);
        out.push(item);
    }
    return out;
}

export function MediaLibraryModal({
    open,
    onOpenChange,
    mode = "multiple",
    value = EMPTY_MEDIA,
    onChange,
}: MediaLibraryModalProps) {
    const queryClient = useQueryClient();
    const [search, setSearch] = useState("");
    const [type, setType] = useState<string>("");
    const scrollRef = useRef<HTMLDivElement>(null);
    const wasOpenRef = useRef(false);

    // Responsive grid columns + virtual row height (avoid 5-col microscopic cells on phones)
    const [gridColumns, setGridColumns] = useState(5);
    useEffect(() => {
        const update = () => {
            const w = window.innerWidth;
            if (w < 380) setGridColumns(2);
            else if (w < 520) setGridColumns(3);
            else if (w < 900) setGridColumns(4);
            else setGridColumns(5);
        };
        update();
        window.addEventListener("resize", update);
        return () => window.removeEventListener("resize", update);
    }, []);

    // Selection state
    const [selected, setSelected] = useState<MediaAsset[]>([]);

    useEffect(() => {
        // Sync incoming value only on open transition, not on every render while open.
        // This prevents stale parent state from re-selecting an item user just unselected.
        if (open && !wasOpenRef.current) {
            setSelected(normalizeSelection(value));
        }
        if (!open) {
            setSelected([]);
        }
        wasOpenRef.current = open;
    }, [open, value]);

    // Fetch
    const {
        data,
        fetchNextPage,
        hasNextPage,
        isFetchingNextPage,
        isLoading
    } = useInfiniteQuery({
        queryKey: ["media", { search, type }],
        queryFn: async ({ pageParam }) => {
            return api.listMedia({ cursor: pageParam as string | undefined, search, type, limit: 50 });
        },
        initialPageParam: null as string | null,
        getNextPageParam: (lastPage) => lastPage.next_cursor || null,
        enabled: open,
    });

    const allItems = data?.pages.flatMap((p) => p.items) ?? [];

    const rowCount = Math.ceil(allItems.length / gridColumns);
    const virtualizer = useVirtualizer({
        count: hasNextPage ? rowCount + 1 : rowCount,
        getScrollElement: () => scrollRef.current,
        estimateSize: () => (gridColumns <= 3 ? 152 : 180),
        overscan: 5,
    });

    useEffect(() => {
        if (!open) return;
        const frame = requestAnimationFrame(() => {
            virtualizer.measure();
        });
        return () => cancelAnimationFrame(frame);
    }, [open, allItems.length, gridColumns, virtualizer]);

    // Handle uploading
    const { mutateAsync: uploadItem, isPending: isUploading } = useMutation({
        mutationFn: async (file: File) => {
            return await api.uploadMedia(file);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["media"] });
        }
    });

    const onDrop = useCallback(
        async (acceptedFiles: File[]) => {
            for (const file of acceptedFiles) {
                try {
                    const res = await uploadItem(file);
                    if (mode === "single" && acceptedFiles.length === 1) {
                        setSelected([res]);
                    } else {
                        setSelected(prev => {
                            if (prev.some(a => a.id === res.id)) return prev;
                            return [...prev, res];
                        });
                    }
                } catch (err) {
                    console.error("Upload failed", err);
                }
            }
        },
        [uploadItem, mode]
    );

    const { getRootProps, getInputProps, isDragActive, open: openFileDialog } = useDropzone({
        onDrop,
        accept: { "image/*": [], "video/*": [] },
        noClick: true, // Specific drop zone only
    });

    const toggleSelect = (asset: MediaAsset) => {
        const matches = (a: MediaAsset) => a.id === asset.id || (!!a.url && !!asset.url && a.url === asset.url);
        if (mode === "single") {
            setSelected((prev) => (prev.some(matches) ? [] : [asset]));
        } else {
            setSelected((prev) => {
                const isSelected = prev.some(matches);
                if (isSelected) {
                    return prev.filter((a) => !matches(a));
                }
                return [...prev, asset];
            });
        }
    };

    const handleDone = () => {
        const normalized = normalizeSelection(selected);
        onChange?.(mode === "single" ? normalized.slice(0, 1) : normalized);
        onOpenChange(false);
    };

    // The sidebar active item
    const activeEditingItem = selected.length === 1 ? selected[0] : null;

    return (
        <Dialog.Root open={open} onOpenChange={onOpenChange}>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
                <Dialog.Content className="fixed inset-0 z-50 flex max-h-[100dvh] flex-col overflow-hidden rounded-none border-0 border-[var(--border)] bg-[var(--background)] shadow-2xl sm:inset-3 sm:rounded-2xl sm:border md:inset-auto md:left-1/2 md:top-1/2 md:h-[min(85vh,100dvh)] md:w-[min(1200px,calc(100%-2rem))] md:max-h-[85vh] md:-translate-x-1/2 md:-translate-y-1/2 md:rounded-2xl md:border">
                    <Dialog.Title className="sr-only">Media Library</Dialog.Title>
                    <Dialog.Description className="sr-only">Manage your media assets</Dialog.Description>

                    {/* Header */}
                    <div className="flex min-h-12 shrink-0 items-center justify-between gap-2 border-b border-[var(--border)] px-3 py-2 pt-[max(0.5rem,env(safe-area-inset-top))] sm:h-16 sm:px-6 sm:py-0 sm:pt-0">
                        <h2 className="truncate text-base font-semibold sm:text-lg">Media</h2>
                        <div className="flex shrink-0 items-center gap-1.5 sm:gap-2">
                            <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
                            <Button size="sm" className="min-w-[4.5rem] md:min-w-24" onClick={handleDone}>
                                Done{selected.length > 0 && ` (${selected.length})`}
                            </Button>
                        </div>
                    </div>

                    <div className="flex min-h-0 flex-1 flex-col overflow-hidden md:flex-row" {...getRootProps()}>
                        <input {...getInputProps()} />

                        {/* Main Content Area */}
                        <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden md:min-w-0">

                            {/* Drag overlay */}
                            {isDragActive && (
                                <div className="absolute inset-0 z-10 flex flex-col items-center justify-center bg-[var(--primary)]/10 backdrop-blur-[2px]">
                                    <UploadCloud className="size-16 text-[var(--primary)] animate-bounce" />
                                    <p className="mt-4 font-semibold text-lg text-[var(--primary)]">Drop files to upload</p>
                                </div>
                            )}

                            {/* Toolbar: upload first in column layout; search | filter | upload on md+ */}
                            <div className="flex shrink-0 flex-col gap-2 border-b border-[var(--border)] bg-[var(--muted)]/30 p-3 md:flex-row md:items-center md:gap-4 md:p-4">
                                <div className="order-1 flex w-full items-center gap-2 md:order-3 md:ml-auto md:w-auto">
                                    <Button
                                        type="button"
                                        onClick={() => openFileDialog()}
                                        variant="default"
                                        className="min-w-0 flex-1 gap-2 md:min-w-0 md:flex-initial"
                                    >
                                        <UploadCloud className="size-4 shrink-0" />
                                        {isUploading ? "Uploading…" : "Upload"}
                                    </Button>
                                    {isUploading && <Loader2 className="size-5 shrink-0 animate-spin text-[var(--muted-foreground)]" />}
                                </div>
                                <div className="relative order-2 min-w-0 flex-1 md:order-1 md:max-w-sm">
                                    <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--muted-foreground)]" />
                                    <Input
                                        placeholder="Search…"
                                        className="bg-white pl-9 dark:bg-black"
                                        value={search}
                                        onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)}
                                    />
                                </div>
                                <select
                                    className="order-3 h-10 w-full shrink-0 rounded-md border border-[var(--border)] bg-transparent px-3 text-sm md:order-2 md:h-9 md:w-auto md:py-0"
                                    value={type}
                                    onChange={(e) => setType(e.target.value)}
                                >
                                    <option value="">All</option>
                                    <option value="image">Images</option>
                                    <option value="video">Videos</option>
                                </select>
                            </div>

                            {/* Grid Area */}
                            <div
                                ref={scrollRef}
                                className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-2 sm:p-4"
                                onScroll={() => {
                                    if (virtualizer.scrollElement) {
                                        const { scrollTop, scrollHeight, clientHeight } = virtualizer.scrollElement;
                                        if (scrollHeight - scrollTop - clientHeight < 100 && hasNextPage && !isFetchingNextPage) {
                                            fetchNextPage();
                                        }
                                    }
                                }}
                            >
                                {isLoading ? (
                                    <div className="flex h-full items-center justify-center">
                                        <Loader2 className="size-8 animate-spin text-[var(--muted-foreground)]" />
                                    </div>
                                ) : allItems.length === 0 ? (
                                    <div className="flex h-full flex-col items-center justify-center text-center">
                                        <ImageIcon className="mb-4 size-16 text-[var(--muted-foreground)]/30" />
                                        <h3 className="text-lg font-medium">No media found</h3>
                                        <p className="text-sm text-[var(--muted-foreground)] mb-4">Drag & drop files here to upload.</p>
                                        <Button onClick={() => openFileDialog()}>Choose Files</Button>
                                    </div>
                                ) : (
                                    <div
                                        style={{
                                            height: `${virtualizer.getTotalSize()}px`,
                                            width: '100%',
                                            position: 'relative',
                                        }}
                                    >
                                        {virtualizer.getVirtualItems().map((virtualRow) => {
                                            return (
                                                <div
                                                    key={virtualRow.index}
                                                    style={{
                                                        position: 'absolute',
                                                        top: 0,
                                                        left: 0,
                                                        width: '100%',
                                                        height: `${virtualRow.size}px`,
                                                        transform: `translateY(${virtualRow.start}px)`,
                                                    }}
                                                    className={cn(
                                                        "grid gap-2 sm:gap-3 md:gap-4",
                                                        gridColumns === 2 && "grid-cols-2",
                                                        gridColumns === 3 && "grid-cols-3",
                                                        gridColumns === 4 && "grid-cols-4",
                                                        gridColumns === 5 && "grid-cols-5"
                                                    )}
                                                >
                                                    {/* Render columns for this row */}
                                                    {Array.from({ length: gridColumns }).map((_, colIndex) => {
                                                        const itemIndex = virtualRow.index * gridColumns + colIndex;
                                                        const asset = allItems[itemIndex];
                                                        if (!asset) return <div key={colIndex} />;

                                                        const isSel = !!selected.find(a => a.id === asset.id || (!!a.url && !!asset.url && a.url === asset.url));

                                                        return (
                                                            <div
                                                                key={asset.id}
                                                                onClick={() => toggleSelect(asset)}
                                                                className={cn(
                                                                    "group relative aspect-square cursor-pointer overflow-hidden rounded-lg border-2 transition-all sm:rounded-xl",
                                                                    isSel
                                                                        ? "border-[var(--primary)] ring-2 ring-[var(--primary)]/30 ring-offset-2 ring-offset-[var(--background)]"
                                                                        : "border-[var(--border)]/60 opacity-80 hover:opacity-100 hover:border-[var(--border)]"
                                                                )}
                                                            >
                                                                {asset.mime_type.startsWith("video/") ? (
                                                                    <div className="flex h-full w-full items-center justify-center bg-[var(--muted)]">
                                                                        <FileVideo className="size-10 text-[var(--muted-foreground)]" />
                                                                    </div>
                                                                ) : (
                                                                    <img
                                                                        src={publicUploadUrl(asset.url)}
                                                                        alt={asset.alt || ""}
                                                                        className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
                                                                        loading="lazy"
                                                                    />
                                                                )}

                                                                {/* Selection Checkmark */}
                                                                <div className={cn(
                                                                    "absolute right-2 top-2 rounded-full bg-white shadow-sm transition-opacity",
                                                                    isSel ? "opacity-100" : "opacity-0 group-hover:opacity-70"
                                                                )}>
                                                                    <CheckCircle2 className={cn("size-6", isSel ? "text-[var(--primary)]" : "text-gray-400")} />
                                                                </div>
                                                                <div
                                                                    className={cn(
                                                                        "absolute inset-x-0 bottom-0 px-2 py-1 text-[10px] font-medium",
                                                                        isSel
                                                                            ? "bg-[var(--primary)]/90 text-[var(--primary-foreground)]"
                                                                            : "bg-black/45 text-white opacity-0 group-hover:opacity-100"
                                                                    )}
                                                                >
                                                                    {isSel ? "Selected" : "Click to select"}
                                                                </div>
                                                            </div>
                                                        );
                                                    })}
                                                </div>
                                            );
                                        })}
                                    </div>
                                )}
                            </div>
                        </div>

                        {/* Editing Sidebar */}
                        {(mode === "single" || selected.length === 1) && activeEditingItem ? (
                            <MediaSidebar
                                asset={activeEditingItem}
                                onUpdate={(updated) => {
                                    setSelected(prev => prev.map(a => a.id === updated.id ? updated : a));
                                    queryClient.invalidateQueries({ queryKey: ["media"] });
                                }}
                            />
                        ) : selected.length > 1 ? (
                            <div className="flex max-h-[38vh] shrink-0 flex-col items-center justify-center border-t border-[var(--border)] bg-[var(--muted)]/10 px-4 py-5 text-center md:h-auto md:max-h-none md:w-80 md:border-l md:border-t-0 md:px-6 md:py-6">
                                <div className="mb-3 rounded-full bg-[var(--primary)]/10 p-4 md:mb-4 md:p-6">
                                    <Tags className="size-8 text-[var(--primary)] md:size-10" />
                                </div>
                                <h3 className="text-base font-semibold md:text-lg">{selected.length} selected</h3>
                                <p className="mt-1 text-xs text-[var(--muted-foreground)] md:mt-2 md:text-sm">
                                    Edit details when only one asset is selected.
                                </p>
                            </div>
                        ) : null}

                    </div>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

// ─── Sub-components ──────────────────────────────────────────────────────────

function MediaSidebar({ asset, onUpdate }: { asset: MediaAsset; onUpdate: (a: MediaAsset) => void }) {
    const [alt, setAlt] = useState(asset.alt || "");
    const [tags, setTags] = useState(asset.tags?.join(", ") || "");
    const { mutateAsync, isPending } = useMutation({
        mutationFn: async () => {
            const tagsArr = tags.split(",").map(t => t.trim()).filter(Boolean);
            await api.patchMedia(asset.id, { alt, tags: tagsArr });
            return { ...asset, alt, tags: tagsArr };
        },
        onSuccess: (updated) => onUpdate(updated),
    });

    return (
        <div className="flex max-h-[42vh] w-full shrink-0 flex-col overflow-y-auto border-t border-[var(--border)] bg-[var(--muted)]/10 pb-[env(safe-area-inset-bottom)] md:max-h-none md:h-full md:w-80 md:border-l md:border-t-0 md:pb-0">
            <div className="shrink-0 border-b border-[var(--border)] px-4 py-2 text-xs font-medium uppercase tracking-wider text-[var(--muted-foreground)] sm:p-4 sm:text-sm">
                Details
            </div>
            <div className="space-y-4 break-words px-4 py-3 sm:p-6">
                {asset.mime_type.startsWith("video/") ? (
                    <video
                        src={publicUploadUrl(asset.url)}
                        controls
                        className="max-h-[28vh] w-full rounded-lg bg-black object-contain md:max-h-none md:rounded-xl aspect-video"
                    />
                ) : (
                    <img
                        src={publicUploadUrl(asset.url)}
                        alt={asset.alt}
                        className="mx-auto max-h-[min(28vh,12rem)] w-full rounded-lg bg-[var(--muted)] object-contain md:max-h-[min(70vh,24rem)]"
                    />
                )}

                <div className="mt-4 space-y-4 sm:mt-6">
                    <div className="text-sm">
                        <p className="text-[var(--muted-foreground)]">ID</p>
                        <p className="font-mono text-xs text-[var(--primary)] break-all">{asset.id}</p>
                    </div>

                    <div className="text-sm">
                        <p className="text-[var(--muted-foreground)]">Size</p>
                        <p>{Math.round((asset.size_bytes || 0) / 1024)} KB</p>
                    </div>

                    <div className="text-sm">
                        <p className="text-[var(--muted-foreground)]">Uploaded</p>
                        <p>{new Date(asset.created_at || "").toLocaleDateString()}</p>
                    </div>

                    <div className="border-t border-[var(--border)] pt-4" />

                    <div className="space-y-2">
                        <label className="text-sm font-medium">Alt Text</label>
                        <Input value={alt} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setAlt(e.target.value)} placeholder="Describe the image..." />
                    </div>

                    <div className="space-y-2">
                        <label className="text-sm font-medium">Tags (comma separated)</label>
                        <Input value={tags} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setTags(e.target.value)} placeholder="summer, hero, banner..." />
                    </div>

                    <Button
                        className="w-full"
                        disabled={isPending || (alt === (asset.alt || "") && tags === (asset.tags?.join(", ") || ""))}
                        onClick={() => mutateAsync()}
                    >
                        {isPending ? "Saving..." : "Save Changes"}
                    </Button>

                    {/* A delete button could go here too! */}
                    <div className="pt-4 sm:pt-8">
                        <Button variant="outline" className="w-full text-red-500 hover:text-red-600 hover:bg-red-50" onClick={async () => {
                            if (confirm("Are you sure you want to delete this asset?")) {
                                await api.deleteMedia(asset.id);
                                onUpdate({} as MediaAsset); // Trigger refresh
                            }
                        }}>
                            Delete Asset
                        </Button>
                    </div>
                </div>
            </div>
        </div>
    );
}
