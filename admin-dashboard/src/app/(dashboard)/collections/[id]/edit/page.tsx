"use client";

import { useParams } from "next/navigation";
import { CollectionEditor } from "@/components/collections/collection-editor";

export default function EditCollectionPage() {
    const params = useParams<{ id: string }>();
    return <CollectionEditor collectionId={params.id} />;
}
