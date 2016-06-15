// @flow

import React from "react";
import {Hero, Heading, FlexContainer, Tabs, TabItem, Affix} from "sourcegraph/components";
import CSSModules from "react-css-modules";
import base from "sourcegraph/components/styles/_base.css";
import styles from "./styles/StyleguideContainer.css";
import ComponentsContainer from "./ComponentsContainer";

class StyleguideContainer extends React.Component {

	render() {
		return (
			<div styleName="bg-near-white">
				<Hero color="purple" pattern="objects">
					<Heading level="2" color="white">The Graph Guide</Heading>
					<p style={{maxWidth: "560px"}} className={base.center}>
						Welcome to the Graph Guide – a living guide to Sourcegraph's brand identity, voice, visual style, and approach to user experience and user interfaces.
					</p>
				</Hero>
				<FlexContainer styleName="container-fixed">
					<Affix offset={20} style={{flex: "0 0 220px"}} className={base.orderlast}>
						<Tabs direction="vertical" color="purple" className={base.ml5}>
							<TabItem>
								<a href="#principles">Principles</a>
							</TabItem>

							<Heading level="5" className={base.mt4}>Brand</Heading>
							<TabItem>
								<a href="#brand-voice">Voice</a>
							</TabItem>
							<TabItem>Logo and Logotype</TabItem>
							{/* <TabItem>Colors</TabItem>
							<TabItem>Typography</TabItem>}

							{/*  <Heading level="5" className={base.mt4}>Utilities</Heading>
							<TabItem>Padding</TabItem>
							<TabItem>Margin</TabItem>
							<TabItem>Colors</TabItem>
							<TabItem>Layout</TabItem>*/}

							<Heading level="5" className={base.mt4}>Layout Components</Heading>
							<TabItem>
								<a href="#layout-flexcontainer">FlexContainer</a>
							</TabItem>
							<TabItem>Affix</TabItem>

							<Heading level="5" className={base.mt4}>UI Components</Heading>
							<TabItem>
								<a href="#components-headings">Headings</a>
							</TabItem>
							<TabItem>
								<a href="#components-buttons">Buttons</a>
							</TabItem>
							<TabItem>
								<a href="#components-tabs">Tabs</a>
							</TabItem>
							<TabItem>
								<a href="#components-panels">Panels</a>
							</TabItem>
							<TabItem>
								<a href="#components-stepper">Stepper</a>
							</TabItem>
							<TabItem>
								<a href="#components-checklists">Checklist Items</a>
							</TabItem>
							<TabItem>Table</TabItem>
							<TabItem>Code</TabItem>

						</Tabs>
					</Affix>
					<ComponentsContainer />
				</FlexContainer>
			</div>
		);
	}
}

export default CSSModules(StyleguideContainer, styles, {allowMultiple: true});
