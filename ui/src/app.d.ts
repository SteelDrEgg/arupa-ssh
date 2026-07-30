declare global {
	namespace App {}

	interface TerminalDisposable {
		dispose(): void;
	}

	type TerminalTheme = {
		background?: string;
		foreground?: string;
		cursor?: string;
		cursorAccent?: string;
		selectionBackground?: string;
	};

	interface XTermInstance {
		cols: number;
		rows: number;
		options: { theme?: TerminalTheme };
		loadAddon(addon: unknown): void;
		open(element: HTMLElement): void;
		onData(listener: (data: string) => void): TerminalDisposable;
		write(data: string): void;
		clear(): void;
		focus(): void;
		dispose(): void;
	}

	interface FitAddonInstance {
		fit(): void;
	}

	interface Window {
		Terminal: new (options: Record<string, unknown>) => XTermInstance;
		FitAddon: { FitAddon: new () => FitAddonInstance };
	}
}

export {};
