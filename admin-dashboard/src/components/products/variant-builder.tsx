"use client";

import { useState, useCallback, useMemo, useEffect } from "react";
import { Plus, X, GripVertical, ChevronDown, ChevronUp, Image as ImageIcon } from "lucide-react";
import Barcode from "react-barcode";
import { Button, Input, Label } from "@/components/ui/primitives";
import type { ProductOption, ProductVariantDraft, MediaAsset } from "@/types/api";
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
                    sku: existing?.sku || nextSku(),
                    barcode: existing?.barcode,
                    price: existing?.price || basePrice || "0.00",
                    sale_price: existing?.sale_price || baseSalePrice || "",
                    cost: existing?.cost || baseCost || "0.00",
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
                                        {options.map(o => (
                                            o.name && <option key={o.name} value={o.name}>{o.name}</option>
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

                            <div className="flex items-center p-3 border-b text-xs font-semibold text-[var(--muted-foreground)] bg-[var(--muted)]/10">
                                <span className="w-8"></span> {/* Checkbox space */}
                                <span className="flex-1 min-w-[200px]">Variant</span>
                                <span className="w-24 text-right pr-2">Price</span>
                                <span className="w-24 text-right pr-2">Sale Price</span>
                                <span className="w-20 text-right pr-4">Quantity</span>
                                <span className="w-32 pr-2">SKU</span>
                            </div>

                            <div className="divide-y divide-[var(--border)]">
                                {Object.entries(groupedVariants).map(([groupName, groupVariants]) => {
                                    const isExpanded = expandedGroups[groupName];
                                    const firstVariantMedia = groupVariants[0]?.media?.[0];

                                    return (
                                        <div key={groupName} className="flex flex-col">
                                            {/* Parent Row */}
                                            {actualGroupBy && (
                                                <div className="flex items-center p-3 hover:bg-[var(--muted)]/20 transition-colors">
                                                    <div className="w-8 flex items-center justify-center">
                                                        <input type="checkbox" className="rounded border-[var(--border)] text-[var(--primary)] focus:ring-[var(--primary)]" />
                                                    </div>
                                                    <div className="flex-1 flex items-center gap-4 min-w-[200px]">
                                                        <button
                                                            type="button"
                                                            onClick={() => { setMediaTarget({ type: 'group', groupName }); setMediaModalOpen(true); }}
                                                            className="size-11 shrink-0 rounded-md border border-dashed border-[var(--border)] hover:border-[var(--primary)] hover:bg-[var(--primary)]/5 flex items-center justify-center transition-all overflow-hidden bg-[var(--panel)] text-[var(--foreground)] relative group"
                                                        >
                                                            {groupVariants[0]?.media && groupVariants[0].media.length > 0 ? (
                                                                <div className="flex w-full h-full p-0.5 gap-0.5 flex-wrap content-start">
                                                                     {groupVariants[0].media.slice(0, 4).map((m, i) => (
                                                                         <img key={m.id} src={m.url} alt="" className={cn("object-cover rounded-sm", groupVariants[0].media!.length === 1 ? "w-full h-full" : "w-[calc(50%-2px)] h-[calc(50%-2px)]")} />
                                                                     ))}
                                                                     {groupVariants[0].media.length > 4 && (
                                                                         <div className="absolute inset-0 bg-black/40 flex items-center justify-center text-[10px] text-white font-bold">
                                                                             +{groupVariants[0].media.length - 4}
                                                                         </div>
                                                                     )}
                                                                </div>
                                                            ) : (
                                                                <ImageIcon className="size-4 text-[var(--muted-foreground)]" />
                                                            )}
                                                        </button>
                                                        <div className="flex flex-col">
                                                            <span className="font-semibold text-sm">{groupName}</span>
                                                            <button type="button" onClick={() => toggleGroup(groupName)} className="flex items-center gap-1 text-xs text-[var(--muted-foreground)] hover:text-foreground hover:underline">
                                                                {groupVariants.length} variants
                                                                {isExpanded ? <ChevronUp className="size-3" /> : <ChevronDown className="size-3" />}
                                                            </button>
                                                        </div>
                                                    </div>
                                                    <div className="w-24 px-1 text-sm">
                                                        <Input
                                                            className="h-8 text-right text-xs bg-[var(--panel)] text-[var(--foreground)]"
                                                            value={groupVariants[0]?.price ?? ""}
                                                            placeholder="0.00"
                                                            onChange={(e) => updateGroupPrice(groupName, e.target.value)}
                                                        />
                                                    </div>
                                                    <div className="w-24 px-1 text-sm">
                                                        <Input
                                                            className="h-8 text-right text-xs bg-[var(--panel)] text-[var(--foreground)]"
                                                            value={groupVariants[0]?.sale_price ?? ""}
                                                            placeholder="Sale"
                                                            onChange={(e) => updateGroupSalePrice(groupName, e.target.value)}
                                                        />
                                                    </div>
                                                    <div className="w-20 text-right px-4 text-sm font-medium">{groupVariants.reduce((sum, v) => sum + (v.quantity || 0), 0)}</div>
                                                    <div className="w-32"></div>
                                                </div>
                                            )}
 
                                            {/* Child Rows */}
                                            {(!actualGroupBy || isExpanded) && (
                                                <div className={cn("flex flex-col divide-y divide-[var(--border)]/50", actualGroupBy && "bg-[var(--muted)]/5 border-t border-[var(--border)]")}>
                                                    {groupVariants.map((v) => {
                                                        const isOutOfStock = (v.quantity || 0) <= 0;
                                                        return (
                                                            <div key={v._index} className={cn("flex items-center py-2 px-3 hover:bg-[var(--muted)]/20", actualGroupBy && "pl-6", isOutOfStock && "opacity-60 bg-[var(--muted)]/30")}>
                                                                <div className="w-8 flex items-center justify-center">
                                                                    <input type="checkbox" className="rounded border-[var(--border)] text-[var(--primary)] focus:ring-[var(--primary)]" />
                                                                </div>
                                                                <div className="flex-1 flex items-center gap-3 min-w-[200px]">
                                                                    <button
                                                                        type="button"
                                                                        onClick={() => { setMediaTarget({ type: 'variant', index: v._index }); setMediaModalOpen(true); }}
                                                                        className="size-9 shrink-0 rounded-md border border-dashed border-[var(--border)] hover:border-[var(--primary)] hover:bg-[var(--primary)]/5 flex items-center justify-center transition-all overflow-hidden bg-[var(--panel)] text-[var(--foreground)] relative"
                                                                    >
                                                                        {v.media && v.media.length > 0 ? (
                                                                            <div className="flex w-full h-full p-0.5 gap-0.5 flex-wrap content-start">
                                                                                {v.media.slice(0, 4).map((m, i) => (
                                                                                    <img key={m.id} src={m.url} alt="" className={cn("object-cover rounded-sm", v.media!.length === 1 ? "w-full h-full" : "w-[calc(50%-2px)] h-[calc(50%-2px)]")} />
                                                                                ))}
                                                                                {v.media.length > 4 && (
                                                                                    <div className="absolute inset-0 bg-black/40 flex items-center justify-center text-[8px] text-white font-bold">
                                                                                        +{v.media.length - 4}
                                                                                    </div>
                                                                                )}
                                                                            </div>
                                                                        ) : (
                                                                            <ImageIcon className="size-4 text-[var(--muted-foreground)]" />
                                                                        )}
                                                                    </button>
                                                                    <div className="flex flex-wrap gap-1 text-sm">
                                                                        {Object.entries(v.options).filter(([k]) => k !== actualGroupBy).map(([k, val]) => (
                                                                            <span key={k} className="px-2 py-0.5 rounded-full bg-[var(--background)] border text-xs">
                                                                                {val}
                                                                            </span>
                                                                        ))}
                                                                        {Object.entries(v.options).length === 1 && actualGroupBy && (
                                                                            <span className="text-xs text-[var(--muted-foreground)] italic">Default</span>
                                                                        )}
                                                                    </div>
                                                                </div>
                                                                <div className="w-24 px-1">
                                                                    <Input
                                                                        className="h-8 text-right text-xs bg-[var(--panel)] text-[var(--foreground)]"
                                                                        value={v.price}
                                                                        placeholder="0.00"
                                                                        onChange={(e) => updateVariant(v._index, { price: e.target.value })}
                                                                    />
                                                                </div>
                                                                <div className="w-24 px-1">
                                                                    <Input
                                                                        className="h-8 text-right text-xs bg-[var(--panel)] text-[var(--foreground)]"
                                                                        value={v.sale_price || ""}
                                                                        placeholder="Sale"
                                                                        onChange={(e) => updateVariant(v._index, { sale_price: e.target.value })}
                                                                    />
                                                                </div>
                                                                <div className="w-20 px-1">
                                                                    <Input
                                                                        className="h-8 text-right text-xs bg-[var(--panel)] text-[var(--foreground)]"
                                                                        type="number"
                                                                        min="0"
                                                                        value={v.quantity ?? ""}
                                                                        placeholder="0"
                                                                        onChange={(e) => updateVariant(v._index, { quantity: parseInt(e.target.value) || 0 })}
                                                                    />
                                                                </div>
                                                                <div className="w-32 px-1">
                                                                    <Input
                                                                        className="h-8 text-xs bg-[var(--panel)] text-[var(--foreground)]"
                                                                        value={v.sku}
                                                                        placeholder="SKU"
                                                                        onChange={(e) => updateVariant(v._index, { sku: e.target.value })}
                                                                    />
                                                                </div>
                                                                {isOutOfStock && (
                                                                    <div className="ml-2 rounded-full bg-rose-100 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider text-rose-700">
                                                                        X Out
                                                                    </div>
                                                                )}
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

