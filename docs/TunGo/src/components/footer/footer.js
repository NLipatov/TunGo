import React from 'react';
import Link from '@docusaurus/Link';
import Translate from '@docusaurus/Translate';
import Styles from './footer.module.css';

export default function Footer() {
    return (
        <footer className={Styles.footer}>
            <p>
                <Translate id="footer.iconsBy">Icons by</Translate>{' '}
                <Link to="https://openmoji.org/" target="_blank" rel="noopener noreferrer">
                    OpenMoji
                </Link>{' '}
                (CC BY-SA 4.0)
            </p>
            <p>
                <Translate id="footer.builtWith">Built with</Translate>{' '}
                <Link to={"https://docusaurus.io/"} target={"_blank"} rel="noopener noreferrer">
                    Docusaurus
                </Link>
            </p>
            <p>
                ©{new Date().getFullYear()} <Translate id="footer.contributors">TunGo Contributors</Translate>
            </p>
        </footer>
    );
}
