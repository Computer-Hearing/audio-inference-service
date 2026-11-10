// src/projects.ts
//
// Подпроекты. Инференс — общий анализатор звука, а модели в нём относятся
// к разным подпроектам. Подпроект определяется по префиксу имени модели
// до первого "_":
//   traffic_mobilenet_1_0922combf1m_2 -> traffic
//   road_rnn_99k_09915acc_1           -> road
// Последнее число в имени — длина чанка в секундах (см. models/name.go на
// бэкенде).
import { modelLabel } from './api.ts';
import type { Model } from './gen/inference/v1/inference_pb.ts';

// Известные подпроекты — в том же порядке, что и в портале. Подпроекты с
// другими префиксами тоже показываются (после известных, по алфавиту),
// просто под своим префиксом.
const PROJECT_TITLES: Record<string, string> = {
  traffic: 'traffic',
  road: 'road surface',
  siren: 'sirens',
  diag: 'diagnostics',
};
const PROJECT_ORDER = Object.keys(PROJECT_TITLES);

export function projectOf(modelName: string | undefined): string {
  return modelName?.split('_')[0] || 'other';
}

export function projectTitle(project: string): string {
  return PROJECT_TITLES[project] ?? project;
}

/** Короткое имя модели внутри подпроекта: без префикса подпроекта и длины чанка */
export function modelShortName(modelName: string): string {
  return modelName.replace(/^[^_]+_/, '').replace(/_\d+$/, '') || modelName;
}

export interface ProjectGroup {
  id: string;
  title: string;
  models: Model[];
}

/** Разложить модели по подпроектам в стабильном порядке */
export function groupByProject(models: Model[]): ProjectGroup[] {
  const byProject = new Map<string, Model[]>();
  for (const model of models) {
    const id = projectOf(model.name);
    byProject.set(id, [...(byProject.get(id) ?? []), model]);
  }
  const rank = (id: string) => {
    const i = PROJECT_ORDER.indexOf(id);
    return i === -1 ? PROJECT_ORDER.length : i;
  };
  return [...byProject.entries()]
    .sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
    .map(([id, list]) => ({
      id,
      title: projectTitle(id),
      models: [...list].sort((a, b) => modelLabel(a).localeCompare(modelLabel(b))),
    }));
}

// ---------------------------------------------------------------------------
// Подписи классов по подпроекту.
// У road только target_output, у traffic — category_output и target_output.
// Если подпроект не найден — показываем "class N", чтобы UI не ломался.
// ---------------------------------------------------------------------------
export type LabelKind = 'category' | 'target';

const PROJECT_LABELS: Record<string, Partial<Record<LabelKind, string[]>>> = {
  road: {
    target: ['dry_asphalt', 'snow', 'wet_asphalt'],
  },
  traffic: {
    category: ['car', 'emv', 'motorcycle', 'tram', 'truck'],
    target: ['acceleration', 'bell', 'braking', 'horn', 'idling', 'passing', 'siren'],
  },
};

export function labelsFor(modelName: string | undefined, kind: LabelKind, count: number): string[] {
  const labels = PROJECT_LABELS[projectOf(modelName)]?.[kind];
  if (labels) return labels;
  return Array.from({ length: count || 0 }, (_, i) => `class ${i}`);
}
