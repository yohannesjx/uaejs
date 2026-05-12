"use client";

import { useState, useCallback, useMemo, useEffect } from "react";
import { Plus, X, GripVertical, ChevronDown, ChevronUp, Image as ImageIcon } from "lucide-react";
import { Button, Input, Label } from "@/components/ui/primitives";
import type { ProductOption, ProductVariantDraft, MediaAsset } from "@/types/api";
import { publicUploadUrl } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import { MediaLibraryModal } from "@/components/media/media-library-modal";
import {
    DndContext,
    closestCenter,
    KeyboardSensor,
    PointerSensor,
    useSensor,
    useSensors,
    DragEndEvent
} from "@dnd-kit/core";
import {
    arrayMove,
    SortableContext,
    sortableKeyboardCoordinates,
    verticalListSortingStrategy,
    useSortable
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

interface VariantBuilderProps {
    options: ProductOption[];
    onOptionsChange: (options: ProductOption[]) => void;
    variants: ProductVariantDraft[];
    onVariantsChange: (variants: ProductVariantDraft[]) => void;
    nextSku: () => string;
    basePrice?: string;
    baseSalePrice?: string;
    baseCost?: string;
}

const PRESET_OPTIONS = ["Size", "Color", "Material", "Style"];

/** Checkbox | thumb | label | price | sale | qty | sku | cost — same template for header + every row */
const VARIANT_TABLE_GRID =
    "grid w-full grid-cols-[1.5rem_2rem_minmax(4.5rem,1fr)_4.5rem_4.5rem_2.75rem_6.75rem_4.5rem] items-center gap-x-2 px-2 py-1.5";

function pickVariantSku(existing: ProductVariantDraft | undefined, nextSku: () => string): string {
    const raw = existing?.sku;
    if (raw != null && String(raw).trim() !== "") return String(raw).trim();
    return nextSku();
}

function generateMatrix(options: ProductOption[]): Record<string, string>[] {
    const active = options.filter((o) => o.name && o.values.length > 0);
    if (active.length === 0) return [];
    const [first, ...rest] = active;
    const base = first.values.map((v) => ({ [first.name]: v }));
    return rest.reduce<Record<string, string>[]>((acc, option) => {
        return acc.flatMap((combo) =>
            option.values.map((v) => ({ ...combo, [option.name]: v })),
        );
    }, base);
}

function OptionValuesInput({
    values,
    onChange,
}: {
    values: string[];
    onChange: (values: string[]) => void;
}) {
    const [inputValue, setInputValue] = useState("");

    const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === "Enter" || e.key === "," || e.key === " ") {
            e.preventDefault();
            const val = inputValue.trim();
            if (val && !values.includes(val)) {
                onChange([...values, val]);
            }
            setInputValue("");
        } else if (e.key === "Backspace" && !inputValue && values.length > 0) {
            e.preventDefault();
            onChange(values.slice(0, -1));
        }
    };

    const removeValue = (valToRemove: string) => {
        onChange(values.filter((v) => v !== valToRemove));
    };

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const val = e.target.value;
        if (val.includes(",")) {
            const newValues = val
                .split(",")
                .map((v) => v.trim())
                .filter(Boolean);

            const unique = new Set([...values, ...newValues]);
            onChange(Array.from(unique));
            setInputValue("");
        } else {
            setInputValue(val);
        }
    };

    return (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--background)] px-3 py-2 text-sm focus-within:ring-2 focus-within:ring-[var(--ring)]/30">
            {values.map((v) => (
                <span
                    key={v}
                    className="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--muted)] px-2 py-1 text-xs text-[var(--foreground)]"
                >
                    {v}
                    <button
                        type="button"
                        onClick={() => removeValue(v)}
                        className="hover:text-[var(--primary)]"
                    >
                        <X className="size-3" />
                    </button>
                </span>
            ))}
            <input
                type="text"
                value={inputValue}
                onChange={handleChange}
                onKeyDown={handleKeyDown}
                placeholder={values.length === 0 ? "Type value and press Enter, Space or Comma" : ""}
                className="flex-1 bg-transparent min-w-[120px] focus:outline-none"
            />
        </div>
    );
}

function SortableOptionItem({
    id,
    option,
    idx,
    updateOption,
    removeOption
}: {
    id: string;
    option: ProductOption;
    idx: number;
    updateOption: (idx: number, partial: Partial<ProductOption>) => void;
    removeOption: (idx: number) => void;
}) {
    const { attributes, listeners, setNodeRef, transform, transition } = useSortable({ id });
    const style = { transform: CSS.Transform.toString(transform), transition };

    return (
        <div ref={setNodeRef} style={style} className="rounded-lg border border-[var(--border)] bg-[var(--background)] p-3 shadow-sm">
            <div className="mb-2 flex items-center gap-2">
                <button type="button" {...attributes} {...listeners} className="cursor-grab hover:text-foreground touch-none">
                    <GripVertical className="size-4 shrink-0 text-[var(--muted-foreground)]" />
                </button>
                <select
                    value={option.name}
                    onChange={(e) => updateOption(idx, { name: e.target.value })}
                    className="flex-1 rounded-lg border border-[var(--border)] bg-[var(--panel)] px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ring)]/30"
                >
                    <option value="">Custom option</option>
                    {PRESET_OPTIONS.map((p) => (
                        <option key={p} value={p}>
                            {p}
                        </option>
                    ))}
                </select>
                <button
                    type="button"
                    onClick={() => removeOption(idx)}
                    className="rounded-lg p-1 hover:bg-[var(--muted)]"
                >
                    <X className="size-4 text-[var(--muted-foreground)]" />
                </button>
            </div>
            <OptionValuesInput
                values={option.values}
                onChange={(newValues) => updateOption(idx, { values: newValues })}
            />
        </div>
    );
}

export function VariantBuilder({
    options,
    onOptionsChange,
    variants,
    onVariantsChange,
    nextSku,
    basePrice,
    baseSalePrice,
    baseCost,
}: VariantBuilderProps) {
    const [expanded, setExpanded] = useState(true);
    const [groupByOption, setGroupByOption] = useState<string>("");

    const actualGroupBy = options.some(o => o.name === groupByOption) ? groupByOption : (options[0]?.name || "");
    const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});

    const [mediaModalOpen, setMediaModalOpen] = useState(false);
    const [mediaTarget, setMediaTarget] = useState<{ type: 'group', groupName: string } | { type: 'variant', index: number } | null>(null);

    const sensors = useSensors(
        useSensor(PointerSensor),
        useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
    );

    // Keep grouped variant section aligned with current option order.
    // When user reorders options (e.g. Color <-> Size), grouping follows the first option.
    useEffect(() => {
        const firstOptionName = options[0]?.name || "";
        if (firstOptionName && groupByOption !== firstOptionName) {
            setGroupByOption(firstOptionName);
            setExpandedGroups({});
        }
        if (!firstOptionName && groupByOption) {
            setGroupByOption("");
            setExpandedGroups({});
        }
    }, [options, groupByOption]);

    const handleDragEnd = (event: DragEndEvent) => {
        const { active, over } = event;
        if (active.id !== over?.id) {
            const oldIndex = options.findIndex((o, i) => (o.name || `idx-${i}`) === active.id);
            const newIndex = options.findIndex((o, i) => (o.name || `idx-${i}`) === over?.id);

            const next = arrayMove(options, oldIndex, newIndex);
            onOptionsChange(next);
            regenerateVariants(next);
        }
    };

    const toggleGroup = (g: string) => setExpandedGroups(prev => ({ ...prev, [g]: !prev[g] }));

    const isAllExpanded = useMemo(() => {
        if (!actualGroupBy || variants.length === 0) return true;
        const groupNames = Array.from(new Set(variants.map(v => v.options[actualGroupBy] || "Default")));
        return groupNames.every(g => expandedGroups[g] !== false); // default is true if undefined
    }, [variants, actualGroupBy, expandedGroups]);

    const toggleAllGroups = () => {
        if (!actualGroupBy) return;
        const groupNames = Array.from(new Set(variants.map(v => v.options[actualGroupBy] || "Default")));
        const nextState = !isAllExpanded;
        const nextExpandedGroups = { ...expandedGroups };
        groupNames.forEach(g => {
            nextExpandedGroups[g] = nextState;
        });
        setExpandedGroups(nextExpandedGroups);
    };

    const groupedVariants = useMemo(() => {
        const groups: Record<string, (ProductVariantDraft & { _index: number })[]> = {};
        if (variants.length === 0) return groups;

        if (!actualGroupBy) {
            groups["All"] = variants.map((v, i) => ({ ...v, _index: i }));
            return groups;
        }

        variants.forEach((v, i) => {
            const key = v.options[actualGroupBy] || "Default";
            if (!groups[key]) {
                groups[key] = [];
                // Default expand new groups
                if (expandedGroups[key] === undefined) {
                    setExpandedGroups(prev => ({ ...prev, [key]: true }));
                }
            }
            groups[key].push({ ...v, _index: i });
        });
        return groups;
    }, [variants, actualGroupBy, expandedGroups]);

    const currentMedia = useMemo(() => {
        if (!mediaTarget) return [];
        if (mediaTarget.type === 'variant') {
            return variants[mediaTarget.index]?.media || [];
        } else {
            const groupName = mediaTarget.groupName;
            const first = variants.find(v => (v.options[actualGroupBy] === groupName) || (!actualGroupBy && groupName === "All"));
            return first?.media || [];
        }
    }, [mediaTarget, variants, actualGroupBy]);

    const handleMediaChange = (assets: MediaAsset[]) => {
        if (!mediaTarget) return;
        if (mediaTarget.type === 'variant') {
            updateVariant(mediaTarget.index, { media: assets });
        } else if (mediaTarget.type === 'group') {
            const groupName = mediaTarget.groupName;
            const next = variants.map(v => {
                if (v.options[actualGroupBy] === groupName || (!actualGroupBy && groupName === "All")) {
                    return { ...v, media: assets };
                }
                return v;
            });
            onVariantsChange(next);
        }
    };

    const updateGroupPrice = (groupName: string, price: string) => {
        const next = variants.map(v => {
            if (v.options[actualGroupBy] === groupName || (!actualGroupBy && groupName === "All")) {
                return { ...v, price };
            }
            return v;
        });
        onVariantsChange(next);
    };

    const updateGroupSalePrice = (groupName: string, sale_price: string) => {
        const next = variants.map(v => {
            if (v.options[actualGroupBy] === groupName || (!actualGroupBy && groupName === "All")) {
                return { ...v, sale_price };
            }
            return v;
        });
        onVariantsChange(next);
    };

    const updateGroupCost = (groupName: string, costVal: string) => {
        const next = variants.map(v => {
            if (v.options[actualGroupBy] === groupName || (!actualGroupBy && groupName === "All")) {
                return { ...v, cost: costVal };
            }
            return v;
        });
        onVariantsChange(next);
    };

    const addOption = () => {
        const unused = PRESET_OPTIONS.find(
            (p) => !options.some((o) => o.name === p),
        );
        onOptionsChange([...options, { name: unused ?? "", values: [] }]);
    };

    const updateOption = (idx: number, partial: Partial<ProductOption>) => {
        const next = options.map((o, i) => (i === idx ? { ...o, ...partial } : o));
        onOptionsChange(next);
        regenerateVariants(next);
    };

    const removeOption = (idx: number) => {
        const next = options.filter((_, i) => i !== idx);
        onOptionsChange(next);
        regenerateVariants(next);
    };

    const regenerateVariants = useCallback(
        (opts: ProductOption[]) => {
            const matrix = generateMatrix(opts);
            const next = matrix.map((combo, i): ProductVariantDraft => {
                // Try to preserve existing variant properties based on options matching or position
                const existing = variants.find(v => JSON.stringify(v.options) === JSON.stringify(combo)) || variants[i];
                return {
                    id: existing?.id,
                    sku: pickVariantSku(existing, nextSku),
                    barcode: existing?.barcode,
                    price: existing?.price || basePrice || "0.00",
                    sale_price: existing?.sale_price ?? (baseSalePrice && baseSalePrice.trim() !== "" ? baseSalePrice : "") ?? "",
                    cost: existing?.cost ?? (baseCost && baseCost.trim() !== "" ? baseCost : undefined) ?? "",
                    weight_g: existing?.weight_g,
                    quantity: existing?.quantity || 0,
                    media: existing?.media || [],
                    options: combo,
                };
            });
            onVariantsChange(next);
        },
        [variants, nextSku, onVariantsChange, basePrice, baseSalePrice, baseCost],
    );

    const updateVariant = (idx: number, partial: Partial<ProductVariantDraft>) => {
        onVariantsChange(variants.map((v, i) => (i === idx ? { ...v, ...partial } : v)));
    };

    // Ensure every variant has a non-empty SKU (matrix regen / API / sync can leave blanks).
    useEffect(() => {
        if (variants.length === 0) return;
        let needsSku = false;
        const next = variants.map((v) => {
            if (v.sku != null && String(v.sku).trim() !== "") return v;
            needsSku = true;
            return { ...v, sku: nextSku() };
        });
        if (!needsSku) return;
        onVariantsChange(next);
    }, [variants, nextSku, onVariantsChange]);

    return (
        <div className="rounded-xl border border-[var(--border)] bg-[var(--panel)]">
            <button
                type="button"
                onClick={() => setExpanded((e) => !e)}
                className="flex w-full items-center justify-between px-5 py-4 text-sm font-semibold"
            >
                Variants
                {expanded ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
            </button>

            {expanded && (
                <div className="space-y-4 border-t border-[var(--border)] px-5 pb-5 pt-4">
                    {/* Options Array */}
                    <div className="space-y-3">
                        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
                            <SortableContext items={options.map((o, i) => o.name || `idx-${i}`)} strategy={verticalListSortingStrategy}>
                                {options.map((option, idx) => (
                                    <SortableOptionItem
                                        key={option.name || `idx-${idx}`}
                                        id={option.name || `idx-${idx}`}
                                        option={option}
                                        idx={idx}
                                        updateOption={updateOption}
                                        removeOption={removeOption}
                                    />
                                ))}
                            </SortableContext>
                        </DndContext>

                        <Button type="button" variant="outline" size="sm" onClick={addOption}>
                            <Plus className="size-4" />
                            Add option
                        </Button>
                    </div>

                    {/* Grouped Variants View */}
                    {variants.length > 0 && (
                        <div className="mt-8 border rounded-lg overflow-hidden bg-[var(--background)]">
                            {/* Grouping Toolbar */}
                            {options.length > 1 && (
                                <div className="flex items-center gap-3 p-3 border-b border-[var(--border)] bg-[var(--muted)]/20">
                                    <Label className="text-sm font-medium">Group by</Label>
                                    <select
                                        className="rounded-md border border-[var(--border)] bg-transparent px-3 py-1.5 text-sm"
                                        value={actualGroupBy}
                                        onChange={(e) => setGroupByOption(e.target.value)}
                                    >
                                        {options
                                            .filter((o) => o.name)
                                            .map((o) => (
                                                <option key={o.name} value={o.name}>
                                                    {o.name}
                                                </option>
                                            ))}
                                    </select>
                                    <div className="ml-auto">
                                        <button
                                            type="button"
                                            onClick={toggleAllGroups}
                                            className="flex items-center justify-center rounded-lg p-1.5 hover:bg-[var(--border)] text-[var(--muted-foreground)] transition-colors"
                                            title={isAllExpanded ? "Collapse All" : "Expand All"}
                                        >
                                            {isAllExpanded ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
                                        </button>
                                    </div>
                                </div>
                            )}

                            <div
                                className={cn(
                                    VARIANT_TABLE_GRID,
                                    "border-b border-[var(--border)] bg-[var(--muted)]/10 py-2 text-[10px] font-semibold uppercase tracking-wide text-[var(--muted-foreground)]",
                                )}
                            >
                                <span className="justify-self-center" aria-hidden />
                                <span aria-hidden />
                                <span className="min-w-0">Variant</span>
                                <span className="text-right">Price</span>
                                <span className="text-right">Sale</span>
                                <span className="text-right">Qty</span>
                                <span>SKU</span>
                                <span className="text-right">Cost</span>
                            </div>

                            <div className="divide-y divide-[var(--border)]">
                                {Object.entries(groupedVariants).map(([groupName, groupVariants]) => {
                                    const isExpanded = expandedGroups[groupName] !== false;
                                    const compact = groupVariants.length === 1;

                                    if (compact) {
                                        const v = groupVariants[0];
                                        const isOutOfStock = (v.quantity || 0) <= 0;
                                        const optionLabel =
                                            !actualGroupBy || groupName === "All"
                                                ? Object.entries(v.options)
                                                      .map(([, val]) => val)
                                                      .filter(Boolean)
                                                      .join(" · ") || "Variant"
                                                : String(groupName);
                                        return (
                                            <div
                                                key={groupName}
                                                className={cn(
                                                    VARIANT_TABLE_GRID,
                                                    "border-b border-[var(--border)] hover:bg-[var(--muted)]/15",
                                                    isOutOfStock && "bg-[var(--muted)]/25 opacity-70",
                                                )}
                                            >
                                                <div className="flex justify-center">
                                                    <input type="checkbox" className="rounded border-[var(--border)] text-[var(--primary)] focus:ring-[var(--primary)]" />
                                                </div>
                                                <button
                                                    type="button"
                                                    onClick={() => {
                                                        setMediaTarget({ type: "variant", index: v._index });
                                                        setMediaModalOpen(true);
                                                    }}
                                                    className="size-8 shrink-0 justify-self-start overflow-hidden rounded border border-dashed border-[var(--border)] bg-[var(--panel)] hover:border-[var(--primary)] hover:bg-[var(--primary)]/5"
                                                >
                                                    {v.media && v.media.length > 0 ? (
                                                        // eslint-disable-next-line @next/next/no-img-element
                                                        <img src={publicUploadUrl(v.media[0].url)} alt="" className="size-full object-cover" />
                                                    ) : (
                                                        <ImageIcon className="mx-auto size-3.5 text-[var(--muted-foreground)]" />
                                                    )}
                                                </button>
                                                <div className="flex min-w-0 flex-col gap-0.5">
                                                    <span className="truncate text-xs font-medium">{optionLabel}</span>
                                                    {isOutOfStock && (
                                                        <span className="w-fit rounded-full bg-rose-100 px-1.5 py-0.5 text-[9px] font-bold uppercase text-rose-700">
                                                            Out
                                                        </span>
                                                    )}
                                                </div>
                                                <Input
                                                    className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                    value={v.price}
                                                    placeholder="0"
                                                    onChange={(e) => updateVariant(v._index, { price: e.target.value })}
                                                />
                                                <Input
                                                    className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                    value={v.sale_price || ""}
                                                    placeholder="Sale"
                                                    onChange={(e) => updateVariant(v._index, { sale_price: e.target.value })}
                                                />
                                                <Input
                                                    className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                    type="number"
                                                    min="0"
                                                    value={v.quantity ?? ""}
                                                    onChange={(e) => updateVariant(v._index, { quantity: parseInt(e.target.value, 10) || 0 })}
                                                />
                                                <Input
                                                    className="h-7 w-full min-w-0 px-1 font-mono text-[11px]"
                                                    value={v.sku}
                                                    onChange={(e) => updateVariant(v._index, { sku: e.target.value })}
                                                />
                                                <Input
                                                    className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                    value={v.cost ?? ""}
                                                    placeholder="Cost"
                                                    onChange={(e) => updateVariant(v._index, { cost: e.target.value })}
                                                />
                                            </div>
                                        );
                                    }

                                    return (
                                        <div key={groupName} className="flex flex-col">
                                            {actualGroupBy && (
                                                <div
                                                    className={cn(
                                                        VARIANT_TABLE_GRID,
                                                        "border-b border-[var(--border)] hover:bg-[var(--muted)]/20",
                                                    )}
                                                >
                                                    <div className="flex justify-center">
                                                        <input type="checkbox" className="rounded border-[var(--border)] text-[var(--primary)] focus:ring-[var(--primary)]" />
                                                    </div>
                                                    <button
                                                        type="button"
                                                        onClick={() => {
                                                            setMediaTarget({ type: "group", groupName });
                                                            setMediaModalOpen(true);
                                                        }}
                                                        className="size-9 shrink-0 justify-self-start overflow-hidden rounded border border-dashed border-[var(--border)] bg-[var(--panel)] hover:border-[var(--primary)]"
                                                    >
                                                        {groupVariants[0]?.media && groupVariants[0].media.length > 0 ? (
                                                            // eslint-disable-next-line @next/next/no-img-element
                                                            <img
                                                                src={publicUploadUrl(groupVariants[0].media![0].url)}
                                                                alt=""
                                                                className="size-full object-cover"
                                                            />
                                                        ) : (
                                                            <ImageIcon className="mx-auto size-3.5 text-[var(--muted-foreground)]" />
                                                        )}
                                                    </button>
                                                    <div className="min-w-0">
                                                        <span className="block truncate text-xs font-semibold">{groupName}</span>
                                                        <button
                                                            type="button"
                                                            onClick={() => toggleGroup(groupName)}
                                                            className="text-[10px] text-[var(--muted-foreground)] hover:underline"
                                                        >
                                                            {groupVariants.length} variants
                                                            {isExpanded ? <ChevronUp className="ml-0.5 inline size-3" /> : <ChevronDown className="ml-0.5 inline size-3" />}
                                                        </button>
                                                    </div>
                                                    <Input
                                                        className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                        value={groupVariants[0]?.price ?? ""}
                                                        onChange={(e) => updateGroupPrice(groupName, e.target.value)}
                                                    />
                                                    <Input
                                                        className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                        value={groupVariants[0]?.sale_price ?? ""}
                                                        onChange={(e) => updateGroupSalePrice(groupName, e.target.value)}
                                                    />
                                                    <div className="text-right text-xs font-medium tabular-nums text-[var(--muted-foreground)]">
                                                        {groupVariants.reduce((sum, x) => sum + (x.quantity || 0), 0)}
                                                    </div>
                                                    <div className="truncate text-[10px] text-[var(--muted-foreground)]" title="Per-SKU in rows">
                                                        —
                                                    </div>
                                                    <Input
                                                        className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                        value={groupVariants[0]?.cost ?? ""}
                                                        placeholder="Cost"
                                                        onChange={(e) => updateGroupCost(groupName, e.target.value)}
                                                    />
                                                </div>
                                            )}

                                            {(!actualGroupBy || isExpanded) && (
                                                <div
                                                    className={cn(
                                                        "flex flex-col divide-y divide-[var(--border)]/60",
                                                        actualGroupBy && "bg-[var(--muted)]/5",
                                                    )}
                                                >
                                                    {groupVariants.map((v) => {
                                                        const isOutOfStock = (v.quantity || 0) <= 0;
                                                        return (
                                                            <div
                                                                key={v._index}
                                                                className={cn(
                                                                    VARIANT_TABLE_GRID,
                                                                    "hover:bg-[var(--muted)]/15",
                                                                    actualGroupBy && "border-l-2 border-l-[var(--border)]",
                                                                    isOutOfStock && "bg-[var(--muted)]/20 opacity-70",
                                                                )}
                                                            >
                                                                <div className="flex justify-center">
                                                                    <input type="checkbox" className="rounded border-[var(--border)] text-[var(--primary)]" />
                                                                </div>
                                                                <button
                                                                    type="button"
                                                                    onClick={() => {
                                                                        setMediaTarget({ type: "variant", index: v._index });
                                                                        setMediaModalOpen(true);
                                                                    }}
                                                                    className="size-8 shrink-0 justify-self-start overflow-hidden rounded border border-dashed border-[var(--border)] bg-[var(--panel)]"
                                                                >
                                                                    {v.media && v.media.length > 0 ? (
                                                                        // eslint-disable-next-line @next/next/no-img-element
                                                                        <img src={publicUploadUrl(v.media[0].url)} alt="" className="size-full object-cover" />
                                                                    ) : (
                                                                        <ImageIcon className="mx-auto size-3.5 text-[var(--muted-foreground)]" />
                                                                    )}
                                                                </button>
                                                                <div className="flex min-w-0 flex-wrap gap-1">
                                                                    {Object.entries(v.options)
                                                                        .filter(([k]) => k !== actualGroupBy)
                                                                        .map(([k, val]) => (
                                                                            <span key={k} className="rounded border bg-[var(--background)] px-1.5 py-0.5 text-[10px]">
                                                                                {val}
                                                                            </span>
                                                                        ))}
                                                                    {isOutOfStock && (
                                                                        <span className="rounded-full bg-rose-100 px-1.5 py-0.5 text-[9px] font-bold uppercase text-rose-700">
                                                                            Out
                                                                        </span>
                                                                    )}
                                                                </div>
                                                                <Input
                                                                    className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                                    value={v.price}
                                                                    onChange={(e) => updateVariant(v._index, { price: e.target.value })}
                                                                />
                                                                <Input
                                                                    className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                                    value={v.sale_price || ""}
                                                                    onChange={(e) => updateVariant(v._index, { sale_price: e.target.value })}
                                                                />
                                                                <Input
                                                                    className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                                    type="number"
                                                                    min="0"
                                                                    value={v.quantity ?? ""}
                                                                    onChange={(e) => updateVariant(v._index, { quantity: parseInt(e.target.value, 10) || 0 })}
                                                                />
                                                                <Input
                                                                    className="h-7 w-full min-w-0 px-1 font-mono text-[11px]"
                                                                    value={v.sku}
                                                                    onChange={(e) => updateVariant(v._index, { sku: e.target.value })}
                                                                />
                                                                <Input
                                                                    className="h-7 w-full min-w-0 px-1 text-right text-xs tabular-nums"
                                                                    value={v.cost ?? ""}
                                                                    onChange={(e) => updateVariant(v._index, { cost: e.target.value })}
                                                                />
                                                            </div>
                                                        );
                                                    })}
                                                </div>
                                            )}
                                        </div>
                                    );
                                })}
                            </div>
                        </div>
                    )}

                    {options.length === 0 && (
                        <p className="text-sm text-[var(--muted-foreground)] mt-4">
                            Add options like Size or Color to generate a variant matrix.
                        </p>
                    )}
                </div>
            )}

            <MediaLibraryModal
                open={mediaModalOpen}
                onOpenChange={setMediaModalOpen}
                mode="multiple"
                value={currentMedia}
                onChange={handleMediaChange}
            />
        </div>
    );
}

