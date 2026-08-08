import keeperIconUrl from '@/assets/keeper-icon.svg';
import styles from './BrandLink.module.scss';

type BrandLinkProps = {
  className?: string;
};

/** Product mark for the app shell. Not a navigation control (no external URL). */
export function BrandLink({ className = '' }: BrandLinkProps) {
  const markClassName = `${styles.brandLink} ${className}`.trim();

  return (
    <div className={markClassName} aria-label="CPA Usage Keeper">
      <img className={styles.brandMark} src={keeperIconUrl} alt="" aria-hidden="true" />
      <span className={styles.brandWord}>KEEPER</span>
    </div>
  );
}
