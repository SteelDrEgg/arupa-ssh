export type Theme = 'light' | 'dark';

export const THEME_STORAGE_KEY = 'arupa.theme';

function storedTheme(): Theme | null {
	try {
		const value = window.localStorage.getItem(THEME_STORAGE_KEY);
		return value === 'dark' || value === 'light' ? value : null;
	} catch {
		return null;
	}
}

function preferredTheme(media: MediaQueryList): Theme {
	return storedTheme() ?? (media.matches ? 'dark' : 'light');
}

export function applyTheme(theme: string): Theme {
	const normalized: Theme = theme === 'dark' ? 'dark' : 'light';
	document.body.classList.remove('light', 'dark');
	document.body.classList.add(normalized);
	document.documentElement.dataset.theme = normalized;
	document.documentElement.style.colorScheme = normalized;
	return normalized;
}

export function initializeTheme(listener?: (theme: Theme) => void): () => void {
	const media = window.matchMedia('(prefers-color-scheme: dark)');
	const update = () => {
		const theme = applyTheme(preferredTheme(media));
		listener?.(theme);
	};
	const handleStorage = (event: StorageEvent) => {
		if (event.key === THEME_STORAGE_KEY || event.key === null) update();
	};
	const handleSystemTheme = () => {
		if (storedTheme() === null) update();
	};

	window.addEventListener('storage', handleStorage);
	media.addEventListener('change', handleSystemTheme);
	update();

	return () => {
		window.removeEventListener('storage', handleStorage);
		media.removeEventListener('change', handleSystemTheme);
	};
}
