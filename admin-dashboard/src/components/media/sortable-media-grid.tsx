"use client";

import { DndContext, closestCenter, KeyboardSensor, PointerSensor, useSensor, useSensors, DragEndEvent } from '@dnd-kit/core';
import { arrayMove, SortableContext, sortableKeyboardCoordinates, rectSortingStrategy, useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { MediaAsset } from '@/types/api';
import { X } from 'lucide-react';

interface SortableMediaGridProps {
    items: MediaAsset[];
    onChange: (items: MediaAsset[]) => void;
}

export function SortableMediaGrid({ items, onChange }: SortableMediaGridProps) {
    const sensors = useSensors(
        useSensor(PointerSensor, {
            activationConstraint: {
                distance: 5,
            },
        }),
        useSensor(KeyboardSensor, {
            coordinateGetter: sortableKeyboardCoordinates,
        })
    );

    function handleDragEnd(event: DragEndEvent) {
        const { active, over } = event;
        if (over && active.id !== over.id) {
            const oldIndex = items.findIndex((item) => item.id === active.id);
            const newIndex = items.findIndex((item) => item.id === over.id);
            onChange(arrayMove(items, oldIndex, newIndex));
        }
    }

    if (!items || items.length === 0) return null;

    return (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext items={items.map(i => i.id)} strategy={rectSortingStrategy}>
                <div className="mb-3 grid grid-cols-4 gap-2 sm:grid-cols-6">
                    {items.map((m) => (
                        <SortableItem key={m.id} item={m} onRemove={(id) => onChange(items.filter(i => i.id !== id))} />
                    ))}
                </div>
            </SortableContext>
        </DndContext>
    );
}

function SortableItem({ item, onRemove }: { item: MediaAsset; onRemove: (id: string) => void }) {
    const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id });
    const style = { transform: CSS.Transform.toString(transform), transition, zIndex: isDragging ? 2 : 1 };

    return (
        <div ref={setNodeRef} style={style} {...attributes} {...listeners} className={`relative group aspect-square overflow-hidden rounded-lg border border-[var(--border)] cursor-grab bg-[var(--muted)] ${isDragging ? "opacity-50 ring-2 ring-[var(--primary)]" : "hover:border-[var(--primary)]"}`}>
            <img src={item.url} alt={item.alt || ""} className="h-full w-full object-cover" draggable={false} />
            <button
                type="button"
                onClick={(e) => { e.stopPropagation(); onRemove(item.id); }}
                className="absolute top-1 right-1 rounded-full bg-black/50 p-1 text-white opacity-0 transition-opacity hover:bg-black group-hover:opacity-100 cursor-pointer pointer-events-auto"
                onPointerDown={(e) => e.stopPropagation()} // Prevent drag start when clicking remove
            >
                <X className="size-3" />
            </button>
        </div>
    );
}
