import { useReducer } from 'react';
import type {
  CatalogBlock,
  CatalogKind,
  FieldShortcut,
  TemplateBlock,
  TemplateDetail,
  TemplateLayout,
  TextSize,
} from '../../../services/documents';
import {
  cloneLayout,
  createBlock,
  insertAfter,
  insertAt,
  moveBlock,
  removeBlock,
  sameLayout,
  updateBlock,
  updateBlockOption,
} from './draft';

// Estado del editor: la plantilla tal como la guardó Go (base) y el borrador local. El borrador
// nunca se sustituye por una lectura del servidor sin que la persona lo decida (conflicto de
// revisión, aviso de cambios de otra persona): solo «Descartar mi borrador» lo reemplaza.

export interface RemovedBlock {
  block: TemplateBlock;
  index: number;
}

export interface DesignerState {
  base: TemplateDetail;
  draft: TemplateLayout;
  selectedID: string | null;
  removed: RemovedBlock | null;
}

type Action =
  | { type: 'select'; id: string | null }
  | {
      type: 'add';
      definition: CatalogBlock;
      kind: CatalogKind;
      shortcut?: FieldShortcut;
      index?: number;
    }
  | { type: 'remove'; id: string }
  | { type: 'undo-remove' }
  | { type: 'move'; from: number; to: number }
  | { type: 'update'; id: string; change: Partial<Omit<TemplateBlock, 'id' | 'type'>> }
  | { type: 'option'; id: string; key: string; value: unknown }
  | { type: 'text-size'; value: TextSize }
  // Go confirmó una versión nueva: la base cambia y el borrador pasa a ser lo guardado.
  | { type: 'saved'; template: TemplateDetail }
  // Otra lectura de la plantilla (renombre, activación) que no toca el borrador.
  | { type: 'base'; template: TemplateDetail }
  // Descartar el borrador y partir de la plantilla indicada.
  | { type: 'reset'; template: TemplateDetail };

function withBlocks(state: DesignerState, blocks: TemplateBlock[]): DesignerState {
  return { ...state, draft: { ...state.draft, blocks } };
}

function reducer(state: DesignerState, action: Action): DesignerState {
  const blocks = state.draft.blocks;
  switch (action.type) {
    case 'select':
      return { ...state, selectedID: action.id };
    case 'add': {
      const block = createBlock(action.definition, action.kind, blocks, action.shortcut);
      const next =
        action.index === undefined
          ? insertAfter(blocks, block, state.selectedID)
          : insertAt(blocks, block, action.index);
      return { ...withBlocks(state, next), selectedID: block.id, removed: null };
    }
    case 'remove': {
      const index = blocks.findIndex((block) => block.id === action.id);
      const block = blocks[index];
      if (!block) {
        return state;
      }
      // La selección pasa al bloque vecino para seguir editando sin buscarlo.
      const remaining = removeBlock(blocks, action.id);
      const neighbour = remaining[Math.min(index, remaining.length - 1)];
      return {
        ...withBlocks(state, remaining),
        selectedID: neighbour?.id ?? null,
        removed: { block, index },
      };
    }
    case 'undo-remove': {
      if (!state.removed) {
        return state;
      }
      const restored = insertAt(blocks, state.removed.block, state.removed.index);
      return { ...withBlocks(state, restored), selectedID: state.removed.block.id, removed: null };
    }
    case 'move':
      return { ...withBlocks(state, moveBlock(blocks, action.from, action.to)), removed: null };
    case 'update':
      return withBlocks(state, updateBlock(blocks, action.id, action.change));
    case 'option':
      return withBlocks(state, updateBlockOption(blocks, action.id, action.key, action.value));
    case 'text-size':
      return { ...state, draft: { ...state.draft, options: { text_size: action.value } } };
    case 'saved':
      return {
        ...state,
        base: action.template,
        draft: cloneLayout(action.template.layout),
        removed: null,
      };
    case 'base':
      return { ...state, base: { ...action.template, layout: state.base.layout } };
    case 'reset':
      return initial(action.template);
  }
  return state;
}

function initial(template: TemplateDetail): DesignerState {
  return {
    base: template,
    draft: cloneLayout(template.layout),
    selectedID: template.layout.blocks[0]?.id ?? null,
    removed: null,
  };
}

export function useDesignerDraft(template: TemplateDetail) {
  const [state, dispatch] = useReducer(reducer, template, initial);
  const dirty = !sameLayout(state.draft, state.base.layout);
  const selected = state.draft.blocks.find((block) => block.id === state.selectedID) ?? null;
  return { state, dispatch, dirty, selected };
}

export type DesignerDispatch = ReturnType<typeof useDesignerDraft>['dispatch'];
