import React from 'react';
import Link from '@docusaurus/Link';
import styles from './footer.module.css';

export default function Footer() {
    return (
        <footer className={styles.footer}>
            <p>
                Icons by{' '}
                <Link to="https://openmoji.org/" target="_blank" rel="noopener noreferrer">
                    OpenMoji
                </Link>{' '}
                (CC BY-SA 4.0)
            </p>
            <p>
                Built with{' '}
                <Link to={"https://docusaurus.io/"} target={"_blank"} rel="noopener noreferrer">
                    Docusaurus
                </Link>
            </p>
            <p>
                ©{new Date().getFullYear()} TunGo Contributors
            </p>
        </footer>
    );
}
