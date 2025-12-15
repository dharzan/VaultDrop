type StateUpdater<T> = (value: T | ((prev: T) => T)) => void;

declare module "https://esm.sh/react@18.2.0" {
  export type ReactNode = any;
  export type FC<P = {}> = (props: P & { children?: ReactNode }) => ReactElement | null;
  export type ReactElement = any;
  export function useState<T>(initial: T | (() => T)): [T, StateUpdater<T>];
  export function useEffect(effect: () => void | (() => void), deps?: readonly unknown[]): void;
  export function useMemo<T>(factory: () => T, deps?: readonly unknown[]): T;
  export function useCallback<T extends (...args: any[]) => any>(fn: T, deps?: readonly unknown[]): T;
  const React: { createElement: (...args: any[]) => ReactElement };
  export default React;
}

declare module "https://esm.sh/react-dom@18.2.0/client" {
  export function createRoot(container: Element | DocumentFragment): {
    render(children: any): void;
  };
}

declare namespace JSX {
  interface IntrinsicElements {
    [elemName: string]: any;
  }
}
